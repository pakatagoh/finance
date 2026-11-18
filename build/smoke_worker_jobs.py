#!/usr/bin/env python3
"""Validate real worker files and restore a synthetic full database backup.

Only unique temporary containers, an internal network and scratch files are used.
No host ports, production credentials, production backups or live schedules.
"""

import argparse
import json
import os
from pathlib import Path
import tempfile
import time
import uuid

from smoke_container_images import docker, wait_for

ROOT = Path(__file__).resolve().parents[1]
KINDS = {"credit_card", "debit_card", "paynow", "funds_transfer", "incoming_transfer", "reversal"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", default="finance:mise-dockerfile")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    runtime = f"{args.prefix}-runtime"
    backup = f"{args.prefix}-backup"
    suffix = uuid.uuid4().hex[:10]
    network = f"finance-jobs-test-{suffix}"
    database = f"finance-jobs-db-{suffix}"
    owned = []
    network_created = False
    job_mount = ["--mount", f"type=bind,source={ROOT / 'jobs'},target=/etc/finance,readonly"]
    report = {}

    def run_owned(*options: str, expected: int | None = 0):
        name = f"finance-jobs-run-{suffix}-{len(owned)}"
        owned.append(name)
        return docker("run", "--name", name, *options, expected=expected)

    def sql(db, query):
        return docker("exec", database, "psql", "-X", "-U", "finance", "-d", db,
                      "-v", "ON_ERROR_STOP=1", "-Atc", query).stdout.strip()

    def pg_env(db):
        return ["-e", f"PGHOST={database}", "-e", "PGUSER=finance",
                "-e", "PGPASSWORD=finance-jobs-test-only", "-e", f"PGDATABASE={db}"]

    def cron_command(filename):
        lines = (ROOT / "jobs" / filename).read_text().splitlines()
        job = next(line for line in lines if line and line[0].isdigit())
        return job.split(None, 5)[5]

    def snapshot(db):
        tables = sql(db, "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename;").splitlines()
        data = {}
        for table in tables:
            identifier = '"' + table.replace('"', '""') + '"'
            data[table] = json.loads(sql(db,
                "SELECT coalesce(jsonb_agg(record_data ORDER BY record_data::text), '[]'::jsonb)::text "
                f"FROM (SELECT to_jsonb(t) record_data FROM public.{identifier} t) records;"))
        indexes = sql(db, "SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname='public' ORDER BY tablename,indexname;")
        sequence = sql(db, "SELECT last_value,is_called FROM public.goose_db_version_id_seq;")
        constraints = sql(db,
            "SELECT c.relname,k.conname,pg_get_constraintdef(k.oid) FROM pg_constraint k "
            "JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace "
            "WHERE n.nspname='public' ORDER BY c.relname,k.conname;")
        return data, indexes, sequence, constraints

    try:
        cron_results = {}
        for target, filename in [("categorizer", "categorize.crontab"), ("backup", "backup.crontab")]:
            options = ["--rm", "--network", "none", "-e", "TZ=UTC", *job_mount]
            if target == "backup":
                options += ["--tmpfs", "/var/lib/postgresql"]
            output = run_owned(*options, f"{args.prefix}-{target}", "-debug", "-test", f"/etc/finance/{filename}").stdout
            assert "crontab is valid" in output
            assert "TZ: Asia/Singapore" in output
            cron_results[filename] = "valid; Singapore timezone overrides container UTC"
        report["crontabs"] = cron_results

        with tempfile.TemporaryDirectory(prefix="finance-backup-restore-", dir=os.environ.get("TMPDIR")) as directory:
            folder = Path(directory)
            folder.chmod(0o755)
            output = folder / "backups"
            output.mkdir()
            # Test-only shared write access for image UID999; archives still must be
            # 0600. Production mounts should be owned by UID999, not world-writable.
            output.chmod(0o777)
            output_mount = ["--mount", f"type=bind,source={output},target=/backups"]

            docker("network", "create", "--internal", network)
            network_created = True
            owned.append(database)
            docker("run", "-d", "--name", database, "--network", network,
                   "--tmpfs", "/var/lib/postgresql", "-e", "POSTGRES_USER=finance",
                   "-e", "POSTGRES_PASSWORD=finance-jobs-test-only", "-e", "POSTGRES_DB=finance_source",
                   "postgres:18.6-bookworm")
            wait_for(lambda: docker("exec", database, "pg_isready", "-h", "127.0.0.1", "-U", "finance", "-d", "finance_source", expected=None).returncode == 0, "isolated PostgreSQL TCP listener")
            source_url = f"postgres://finance:finance-jobs-test-only@{database}:5432/finance_source?sslmode=disable"
            for command in [("migrate",), ("seed", "initial-categories")]:
                run_owned("--rm", "--network", network, "-e", f"DATABASE_URL={source_url}", runtime, *command)
            sql("finance_source", """
                WITH kinds(kind,direction) AS (VALUES
                    ('credit_card','debit'), ('debit_card','debit'), ('paynow','debit'),
                    ('funds_transfer','debit'), ('incoming_transfer','credit'), ('reversal','credit'))
                INSERT INTO transactions
                    (source_mailbox,gmail_message_id,occurred_at,timestamp_source,source_occurred_text,
                     bank,source_type,kind,direction,currency,amount_minor,notes,category_id,category_source,
                     category_categorized_at,category_attempted_at)
                SELECT 'backup-fixture.invalid',kind,'2026-01-01T00:00:00Z'::timestamptz,
                    'transaction','synthetic timestamp','Test Bank','backup_fixture',kind,direction,
                    'SGD',1234,'Synthetic backup fixture — no personal data',
                    (SELECT id FROM categories ORDER BY id LIMIT 1),'user',
                    '2026-01-01T01:00:00Z'::timestamptz,'2026-01-01T00:30:00Z'::timestamptz
                FROM kinds;
                INSERT INTO category_mappings
                    (normalized_counterparty,kind,direction,currency,category_id,source)
                SELECT 'synthetic merchant','credit_card','debit','SGD',id,'user'
                FROM categories ORDER BY id LIMIT 1;
            """)
            category_result = run_owned(
                "--rm", "--network", network, *job_mount,
                "-e", f"DATABASE_URL={source_url}",
                "-e", "FINANCE_API_TOKEN=finance-jobs-fixture-token",
                "-e", "FINANCE_JEV_ENDPOINT=http://127.0.0.1:9",
                "-e", "FINANCE_JEV_TOKEN=fixture-only", "-e", "FINANCE_JEV_MODEL=fixture-only",
                "--entrypoint", "/bin/sh", f"{args.prefix}-categorizer",
                "-ec", cron_command("categorize.crontab"))
            assert "categorized 0 transactions (0 failed)" in category_result.stdout
            report["categorizer_command_with_no_pending_rows"] = "passed; no AI provider required"
            before = snapshot("finance_source")
            assert {row["kind"] for row in before[0]["transactions"]} == KINDS

            old = output / "finance-old.dump"
            old.write_text("synthetic retention canary")
            age = time.time() - 40 * 86400
            os.utime(old, (age, age))
            unrelated = output / "unrelated-old.dump"
            unrelated.write_text("not owned by Finance backup retention")
            os.utime(unrelated, (age, age))
            backup_options = ["--rm", "--network", network, "--tmpfs", "/var/lib/postgresql", *job_mount, *output_mount]
            result = run_owned(*backup_options, *pg_env("finance_source"), "--entrypoint", "/bin/sh", backup,
                               "-ec", cron_command("backup.crontab"))
            assert "backup created:" in result.stdout
            assert not old.exists() and unrelated.exists()
            archives = list(output.glob("finance-*.dump"))
            assert len(archives) == 1
            archive = archives[0]
            assert archive.stat().st_uid == 999
            assert archive.stat().st_mode & 0o777 == 0o600
            assert not list(output.glob("*.tmp"))

            docker("exec", database, "createdb", "-U", "finance", "finance_restore")
            run_owned(*backup_options, *pg_env("finance_restore"), "--entrypoint", "pg_restore", backup,
                      "--no-password", "--exit-on-error", "--dbname=finance_restore", f"/backups/{archive.name}")
            after = snapshot("finance_restore")
            assert before == after, "Restored rows, indexes, constraints or sequence state differ"
            restored_url = source_url.replace("/finance_source?", "/finance_restore?")
            run_owned("--rm", "--network", network, "-e", f"DATABASE_URL={restored_url}", runtime, "migrate")
            report["restore"] = {
                "transactions": len(after[0]["transactions"]),
                "kinds": sorted({row["kind"] for row in after[0]["transactions"]}),
                "tables_compared": sorted(after[0]),
                "all_rows_indexes_constraints_and_sequence_equal": True,
                "migration_after_restore": "passed",
                "archive_owner": 999, "archive_mode": "0600",
            }

            failure_canary = output / "finance-failure-canary.dump"
            failure_canary.write_text("must survive failed backup")
            os.utime(failure_canary, (age, age))
            previous = set(output.glob("finance-*.dump"))
            failed = run_owned(*backup_options, *pg_env("nonexistent_fixture_database"), "--entrypoint", "/bin/sh",
                               backup, "/etc/finance/backup.sh", expected=None)
            assert failed.returncode != 0
            assert set(output.glob("finance-*.dump")) == previous
            assert not list(output.glob("*.tmp"))
            report["failure_preserves_archives_and_skips_retention"] = True
            report["successful_retention_preserves_unrelated_files"] = True
    finally:
        for name in reversed(owned):
            docker("rm", "-f", "-v", name, expected=None)
        if network_created:
            docker("network", "rm", network)
        for name in owned:
            assert docker("inspect", name, expected=None).returncode != 0, f"Container cleanup failed: {name}"
        if network_created:
            assert docker("network", "inspect", network, expected=None).returncode != 0

    report["cleanup"] = "verified"
    report["production_access"] = False
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
