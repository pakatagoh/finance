"""Static contracts backed by real image smoke tests in smoke_container_images.py."""

from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


def stages():
    text = (ROOT / "Dockerfile").read_text()
    headers = list(re.finditer(r"^FROM\s+(\S+)\s+AS\s+(\S+)\s*$", text, re.MULTILINE))
    return {
        header[2]: (header[1], text[header.end():headers[i + 1].start() if i + 1 < len(headers) else len(text)])
        for i, header in enumerate(headers)
    }


class DockerfileContractTests(unittest.TestCase):
    def test_default_target_preserves_web_runtime(self):
        targets = stages()
        self.assertEqual(list(targets)[-1], "runtime")
        base, body = targets["runtime"]
        self.assertEqual(base, "finance-base")
        self.assertIn('ENTRYPOINT ["/app/finance"]', body)
        self.assertIn('CMD ["serve"]', body)
        self.assertIn("EXPOSE 8080", body)
        self.assertIn("USER 1000:1000", body)
        self.assertIn("/src/static /app/static", body)

    def test_builder_uses_pinned_mise_with_locked_shared_tools(self):
        base, body = stages()["build"]
        self.assertEqual(base, "debian:bookworm-slim")
        self.assertRegex(body, r"COPY --from=ghcr\.io/jdx/mise:2026\.10\.4@sha256:[a-f0-9]{64}")
        self.assertIn("MISE_ENV=build", body)
        self.assertIn("CGO_ENABLED=0", body)
        self.assertIn("GOTOOLCHAIN=local", body)
        self.assertIn("COPY mise.toml mise.lock mise.build.toml mise.build.lock", body)
        self.assertIn("mise install --locked", body)
        self.assertIn("cd /toolchain", body)
        self.assertIn('cp "$(command -v supercronic)" /out/supercronic', body)
        self.assertNotIn("ARG GO_VERSION", (ROOT / "Dockerfile").read_text())
        self.assertNotIn("go install ", body)
        self.assertNotIn("mise.development", body)

    def test_workers_are_separate_and_non_root(self):
        targets = stages()
        self.assertIn("categorizer", targets)
        self.assertIn("backup", targets)
        base, categorizer = targets["categorizer"]
        self.assertEqual(base, "finance-base")
        self.assertIn("USER 1000:1000", categorizer)
        self.assertIn('ENTRYPOINT ["/usr/local/bin/supercronic"]', categorizer)
        self.assertIn('CMD ["/etc/finance/categorize.crontab"]', categorizer)
        base, backup = targets["backup"]
        self.assertEqual(base, "postgres:18.6-bookworm")
        self.assertIn("USER postgres", backup)
        self.assertIn("WORKDIR /backups", backup)
        self.assertIn('ENTRYPOINT ["/usr/local/bin/supercronic"]', backup)
        self.assertIn('CMD ["/etc/finance/backup.crontab"]', backup)
        for body in (categorizer, backup):
            self.assertIn("COPY --from=build /out/supercronic /usr/local/bin/supercronic", body)
            self.assertNotIn("/src/static", body)

    def test_development_configuration_is_excluded_from_build_context(self):
        ignore = (ROOT / ".dockerignore").read_text().splitlines()
        self.assertIn("mise.development.toml", ignore)
        self.assertIn("mise.development.lock", ignore)


if __name__ == "__main__":
    unittest.main()
