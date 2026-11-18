#!/usr/bin/env python3
"""Exercise already-built images using only temporary containers and synthetic data.

No host ports, production credentials or persistent database volumes are used.
Run manually; this intentionally does not match unittest's test_*.py discovery.
"""

import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.request
import uuid


def docker(*args: str, expected: int | None = 0) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        ["docker", *args], text=True, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, timeout=60,
    )
    if expected is not None and result.returncode != expected:
        raise RuntimeError(f"docker {args[0]} failed ({result.returncode}): {result.stdout}")
    return result


def wait_for(check, description):
    for _ in range(40):
        if check():
            return
        time.sleep(0.25)
    raise RuntimeError(f"Timed out waiting for {description}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", default="finance:mise-dockerfile")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    images = {target: f"{args.prefix}-{target}" for target in ("runtime", "categorizer", "backup")}
    checks = []
    suffix = uuid.uuid4().hex[:10]
    network = f"finance-image-smoke-{suffix}"
    database = f"finance-image-db-{suffix}"
    web = f"finance-image-web-{suffix}"
    owned_containers = []
    network_created = False

    def run_owned(*options: str, expected: int | None = 0) -> subprocess.CompletedProcess[str]:
        # --rm only removes a container after it exits. Register its name before
        # starting the CLI so finally also handles attach failures/timeouts.
        name = f"finance-image-run-{suffix}-{len(owned_containers)}"
        owned_containers.append(name)
        return docker("run", "--name", name, *options, expected=expected)

    def run_image(target, *command, entrypoint=None, extra=()):
        options = ["run", "--rm", "--network", "none"]
        if target == "backup":
            options += ["--tmpfs", "/var/lib/postgresql"]
        if entrypoint:
            options += ["--entrypoint", entrypoint]
        return run_owned(*options[1:], *extra, images[target], *command)

    try:
        for target, image in images.items():
            info = json.loads(docker("image", "inspect", image).stdout)[0]
            config = info["Config"]
            assert config["User"] == ("postgres" if target == "backup" else "1000:1000")
            assert config["Entrypoint"] == (["/app/finance"] if target == "runtime" else ["/usr/local/bin/supercronic"])
            expected_cmd = ["serve"] if target == "runtime" else [f"/etc/finance/{'categorize' if target == 'categorizer' else 'backup'}.crontab"]
            assert config["Cmd"] == expected_cmd
            assert not any(value.startswith(("DATABASE_URL=", "FINANCE_API_TOKEN=", "MISE_ENV=")) for value in config.get("Env", []))
            shell = '''test "$(id -u)" != 0
                test -s /etc/ssl/certs/ca-certificates.crt
                sh /usr/local/bin/check-ca-certificates
                test "$(TZ=Asia/Singapore date +%z)" = +0800
                for tool in mise go templ tailwindcss; do
                    if command -v "$tool"; then exit 1; fi
                done
                test ! -e /mise && test ! -e /toolchain && test ! -e /src
            '''
            if target == "runtime":
                shell += '\ntest -s /app/static/css/app.css\n! command -v supercronic\n'
            else:
                shell += '\ntest ! -e /app/static\n'
            if target == "backup":
                shell += '\ntest "$(id -u)" = 999\ntest -w /backups\npg_dump --version\n'
            run_image(target, "-ec", shell, entrypoint="/bin/sh")
            checks.append({"target": target, "image_id": info["Id"], "runtime_contract": "passed"})

        # Files are synthetic and world-readable for the backup image's UID 999.
        with tempfile.TemporaryDirectory(prefix="finance-image-cron-", dir=os.environ.get("TMPDIR")) as directory:
            folder = Path(directory)
            folder.chmod(0o755)
            valid = folder / "valid.crontab"
            valid.write_text("* * * * * /bin/true\n")
            invalid = folder / "invalid.crontab"
            invalid.write_text("not a valid cron schedule\n")
            mounts = ("--mount", f"type=bind,source={folder},target=/fixtures,readonly")
            for target in ("categorizer", "backup"):
                run_image(target, "-test", "/fixtures/valid.crontab", extra=mounts)
                options = ["run", "--rm", "--network", "none", *mounts]
                if target == "backup":
                    options += ["--tmpfs", "/var/lib/postgresql"]
                bad = run_owned(*options[1:], images[target], "-test", "/fixtures/invalid.crontab", expected=None)
                assert bad.returncode != 0, "Invalid crontab must fail closed"
                name = f"finance-image-{target}-{suffix}"
                owned_containers.append(name)
                detached = ["run", "-d", "--name", name, "--network", "none", *mounts]
                if target == "backup":
                    detached += ["--tmpfs", "/var/lib/postgresql"]
                docker(*detached, images[target], "/fixtures/valid.crontab")
                wait_for(lambda: "read crontab" in docker("logs", name).stdout, "scheduler startup")
                docker("stop", "--time", "3", name)
                state = json.loads(docker("inspect", "--format", "{{json .State}}", name).stdout)
                assert state["ExitCode"] == 0, state
                checks.append({"target": target, "cron_parse_and_shutdown": "passed"})

        missing = run_owned(
            "--rm", "--network", "none", "--entrypoint", "/app/finance",
            images["categorizer"], "categorize", expected=None,
        )
        assert missing.returncode != 0 and "DATABASE_URL" in missing.stdout

        docker("network", "create", "--internal", network)
        network_created = True
        owned_containers.append(database)
        docker("run", "-d", "--name", database, "--network", network,
               "--tmpfs", "/var/lib/postgresql", "-e", "POSTGRES_USER=finance",
               "-e", "POSTGRES_PASSWORD=finance-smoke-only", "-e", "POSTGRES_DB=finance_smoke",
               "postgres:18.6-bookworm")
        # The image's init-only temporary server accepts Unix sockets before the
        # final TCP listener exists. Require TCP readiness, not socket readiness.
        wait_for(lambda: docker("exec", database, "pg_isready", "-h", "127.0.0.1", "-U", "finance", "-d", "finance_smoke", expected=None).returncode == 0, "test PostgreSQL TCP listener")
        url = f"postgres://finance:finance-smoke-only@{database}:5432/finance_smoke?sslmode=disable"
        for _ in range(2):
            run_owned("--rm", "--network", network, "-e", f"DATABASE_URL={url}", images["runtime"], "migrate")
        schema = run_owned("--rm", "--network", network, "--tmpfs", "/var/lib/postgresql",
                        "--entrypoint", "pg_dump", images["backup"], "--schema-only", url).stdout
        assert "goose_db_version" in schema
        checks.append({"target": "runtime/backup", "repeat_migrations_and_pg_dump": "passed"})

        owned_containers.append(web)
        docker("run", "-d", "--name", web, "--network", network,
               "-e", f"DATABASE_URL={url}", "-e", "FINANCE_API_TOKEN=finance-image-smoke-token",
               "-e", "APP_ORIGIN=http://localhost:8080", images["runtime"])
        address = docker("inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", web).stdout.strip()

        def ready():
            try:
                with urllib.request.urlopen(f"http://{address}:8080/health/ready", timeout=1) as response:
                    return response.status == 200
            except (OSError, ValueError):
                return False

        wait_for(ready, "isolated web readiness (Linux Docker bridge)")
        for path in ("/health/live", "/health/startup", "/static/css/app.css"):
            with urllib.request.urlopen(f"http://{address}:8080{path}", timeout=3) as response:
                assert response.status == 200
        checks.append({"target": "runtime", "serve_health_and_css": "passed"})
    finally:
        for name in reversed(owned_containers):
            docker("rm", "-f", "-v", name, expected=None)
        if network_created:
            docker("network", "rm", network)
        for name in owned_containers:
            assert docker("inspect", name, expected=None).returncode != 0, f"Container cleanup failed: {name}"
        if network_created:
            assert docker("network", "inspect", network, expected=None).returncode != 0, "Network cleanup failed"

    report = {"checks": checks, "cleanup": "verified", "production_access": False}
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
