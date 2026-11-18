"""Backup safety tests use stub clients; real PostgreSQL restore is tested separately."""

from pathlib import Path
import fcntl
import os
import signal
import subprocess
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "jobs/backup.sh"


class BackupScriptTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="finance-backup-unit-", dir=os.environ.get("TMPDIR"))
        self.addCleanup(self.temp.cleanup)
        self.folder = Path(self.temp.name)
        self.backups = self.folder / "backups"
        self.backups.mkdir()
        self.bin = self.folder / "bin"
        self.bin.mkdir()
        self.env = {**os.environ, "PATH": f"{self.bin}:{os.environ.get('PATH', '/usr/bin:/bin')}",
                    "PGHOST": "unit-test.invalid", "PGUSER": "test", "PGDATABASE": "test",
                    "FINANCE_BACKUP_DIR": str(self.backups)}
        self.env.pop("FINANCE_BACKUP_RETENTION_DAYS", None)
        self.old = self.backups / "finance-old.dump"
        self.old.write_text("previous known-good archive")
        old_time = time.time() - 40 * 86400
        os.utime(self.old, (old_time, old_time))
        self.stub("pg_restore", "exit 0\n")

    def stub(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\nset -eu\n" + body)
        path.chmod(0o755)

    def run_backup(self):
        self.assertTrue(SCRIPT.is_file(), "backup script must exist")
        return subprocess.run(["/bin/sh", str(SCRIPT)], env=self.env, text=True,
                              capture_output=True, timeout=10)

    def test_dump_failure_preserves_previous_backup_and_removes_partial(self):
        self.stub("pg_dump", '''for arg do
  case "$arg" in --file=*) file=${arg#--file=} ;; esac
done
printf partial > "$file"
echo simulated-dump-failure >&2
exit 1
''')
        result = self.run_backup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("simulated-dump-failure", result.stderr)
        self.assertEqual(self.old.read_text(), "previous known-good archive")
        self.assertEqual(list(self.backups.glob("finance-*.dump")), [self.old])
        self.assertEqual(list(self.backups.glob("*.tmp")), [])


    def good_dump(self):
        self.stub("pg_dump", '''for arg do
  case "$arg" in --file=*) file=${arg#--file=} ;; esac
done
printf synthetic-archive > "$file"
''')

    def test_success_publishes_private_archive_then_prunes_only_owned_old_files(self):
        self.good_dump()
        keep = self.backups / "other-old.dump"
        keep.write_text("unrelated file")
        nested = self.backups / "nested"
        nested.mkdir()
        nested_file = nested / "finance-old.dump"
        nested_file.write_text("nested archive")
        link = self.backups / "finance-link.dump"
        link.symlink_to(keep)
        old_time = time.time() - 40 * 86400
        os.utime(keep, (old_time, old_time))
        os.utime(nested_file, (old_time, old_time))
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("backup created:", result.stdout)
        self.assertFalse(self.old.exists())
        archives = [p for p in self.backups.glob("finance-*.dump") if not p.is_symlink()]
        self.assertEqual(len(archives), 1)
        self.assertEqual(archives[0].read_text(), "synthetic-archive")
        self.assertEqual(archives[0].stat().st_mode & 0o777, 0o600)
        self.assertTrue(keep.exists())
        self.assertTrue(nested_file.exists())
        self.assertTrue(link.is_symlink())
        self.assertEqual(list(self.backups.glob("*.tmp")), [])

    def test_validation_failure_preserves_previous_archive(self):
        self.good_dump()
        self.stub("pg_restore", "echo simulated-validation-failure >&2\nexit 1\n")
        result = self.run_backup()
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.old.exists())
        self.assertEqual(list(self.backups.glob("finance-*.dump")), [self.old])
        self.assertEqual(list(self.backups.glob("*.tmp")), [])

    def test_payload_read_failure_after_valid_toc_does_not_publish_or_prune(self):
        self.good_dump()
        self.stub("pg_restore", '''if [ "$1" = --list ]; then exit 0; fi
echo simulated-payload-failure >&2
exit 1
''')
        result = self.run_backup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("simulated-payload-failure", result.stderr)
        self.assertTrue(self.old.exists())
        self.assertEqual(list(self.backups.glob("finance-*.dump")), [self.old])
        self.assertEqual(list(self.backups.glob("*.tmp")), [])

    def test_invalid_retention_fails_before_dump(self):
        for value in ("0", "-1", "not-a-number", "1; exit 0", "999999999999999999999999"):
            with self.subTest(value=value):
                self.env["FINANCE_BACKUP_RETENTION_DAYS"] = value
                result = self.run_backup()
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("positive integer", result.stderr)
                self.assertTrue(self.old.exists())

    def test_retention_preserves_30_whole_days_and_removes_31_whole_days(self):
        self.good_dump()
        keep = self.backups / "finance-30-days.dump"
        remove = self.backups / "finance-31-days.dump"
        for path, age_days in ((keep, 30.5), (remove, 31.5)):
            path.write_text("retention boundary fixture")
            modified = time.time() - age_days * 86400
            os.utime(path, (modified, modified))
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(keep.exists())
        self.assertFalse(remove.exists())

    def test_retention_failure_keeps_newly_published_and_previous_archives(self):
        self.good_dump()
        self.stub("find", "echo simulated-retention-failure >&2\nexit 1\n")
        result = self.run_backup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("simulated-retention-failure", result.stderr)
        self.assertTrue(self.old.exists())
        new = [path for path in self.backups.glob("finance-*.dump") if path != self.old]
        self.assertEqual(len(new), 1)
        self.assertEqual(new[0].read_text(), "synthetic-archive")
        self.assertEqual(list(self.backups.glob("*.tmp")), [])

    def test_directory_lock_rejects_a_concurrent_backup(self):
        self.good_dump()
        with (self.backups / ".finance-backup.lock").open("a") as handle:
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_backup()
        self.assertEqual(result.returncode, 75, result.stderr)
        self.assertIn("already running", result.stderr)
        self.assertTrue(self.old.exists())
        self.assertEqual(list(self.backups.glob("*.tmp")), [])

    def test_two_backups_in_the_same_second_do_not_overwrite(self):
        self.good_dump()
        self.stub("date", "printf '20260101T000000Z\\n'\n")
        for _ in range(2):
            result = self.run_backup()
            self.assertEqual(result.returncode, 0, result.stderr)
        archives = list(self.backups.glob("finance-*.dump"))
        self.assertEqual(len(archives), 2)
        self.assertNotEqual(archives[0].name, archives[1].name)

    def test_destination_with_spaces(self):
        self.good_dump()
        moved = self.folder / "backups with spaces"
        self.backups.rename(moved)
        self.backups = moved
        self.env["FINANCE_BACKUP_DIR"] = str(moved)
        result = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(list(moved.glob("finance-*.dump"))), 1)

    def test_invalid_destination_fails_without_writes(self):
        for value in ("/", "relative-directory"):
            with self.subTest(value=value):
                self.env["FINANCE_BACKUP_DIR"] = value
                self.assertEqual(self.run_backup().returncode, 2)
                self.assertTrue(self.old.exists())

    def test_sigterm_removes_partial_and_preserves_previous_archive(self):
        marker = self.folder / "started"
        self.env["TEST_DUMP_STARTED"] = str(marker)
        self.stub("pg_dump", '''for arg do
  case "$arg" in --file=*) file=${arg#--file=} ;; esac
done
printf partial > "$file"
touch "$TEST_DUMP_STARTED"
sleep 30
''')
        process = subprocess.Popen(["/bin/sh", str(SCRIPT)], env=self.env,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                   start_new_session=True)
        try:
            for _ in range(100):
                if marker.exists():
                    break
                time.sleep(0.02)
            self.assertTrue(marker.exists(), "dump must start before signal injection")
            os.killpg(process.pid, signal.SIGTERM)
            self.assertNotEqual(process.wait(timeout=5), 0)
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
        self.assertTrue(self.old.exists())
        self.assertEqual(list(self.backups.glob("*.tmp")), [])
        self.assertEqual(list(self.backups.glob("finance-*.dump")), [self.old])


class WorkerCrontabTests(unittest.TestCase):
    def test_existing_schedules_timezones_and_deadlines_are_preserved(self):
        expected = {
            "categorize.crontab": "0 * * * * /usr/bin/timeout --signal=TERM --kill-after=30s 3300 /app/finance categorize",
            "backup.crontab": "15 3 * * * /usr/bin/timeout --signal=TERM --kill-after=30s 3600 /bin/sh /etc/finance/backup.sh",
        }
        for name, job in expected.items():
            with self.subTest(name=name):
                content = (ROOT / "jobs" / name).read_text()
                lines = [line for line in content.splitlines() if line and not line.startswith("#")]
                self.assertEqual(lines, ["CRON_TZ=Asia/Singapore", "TZ=Asia/Singapore", job])
                self.assertNotIn("PGPASSWORD", content)


if __name__ == "__main__":
    unittest.main()
