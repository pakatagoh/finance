"""Guard shared build pins and keep development configuration out of builds.

Run with: python3 -m unittest discover -s build -p 'test_*.py'
"""

from pathlib import Path
import tomllib
import unittest

ROOT = Path(__file__).resolve().parents[1]


def config(name):
    return tomllib.loads((ROOT / name).read_text())


class MiseConfigurationTests(unittest.TestCase):
    def test_shared_config_has_no_development_environment(self):
        shared = config("mise.toml")
        self.assertNotIn("env", shared)
        self.assertEqual(
            set(shared["tools"]),
            {"go", "go:github.com/a-h/templ/cmd/templ", "github:tailwindlabs/tailwindcss"},
        )

    def test_development_overlay_preserves_local_tools_and_tasks(self):
        dev = config("mise.development.toml")
        self.assertEqual(
            set(dev["tools"]),
            {"go:github.com/pressly/goose/v3/cmd/goose", "go:github.com/air-verse/air"},
        )
        self.assertEqual(dev["env"]["APP_ORIGIN"], "http://localhost:8080")
        self.assertIn("DATABASE_URL", dev["env"])
        for task in ("db-up", "db-ready", "db-down", "db-reset", "db-migrate", "seed-categories", "dev", "test-integration", "test"):
            self.assertIn(task, dev["tasks"])
        self.assertNotIn("db-up", config("mise.toml")["tasks"])

    def test_build_overlay_adds_only_scheduler(self):
        build = config("mise.build.toml")
        self.assertEqual(set(build["tools"]), {"go:github.com/aptible/supercronic"})
        self.assertNotIn("env", build)
        self.assertNotIn("tasks", build)

    def test_compiler_generator_and_existing_pipeline_pins_agree(self):
        shared = config("mise.toml")["tools"]
        dev = config("mise.development.toml")["tools"]
        module = (ROOT / "go.mod").read_text()
        dockerfile = (ROOT / "Dockerfile").read_text()
        workflow = (ROOT / ".github/workflows/container.yml").read_text()
        self.assertIn(f"go {shared['go']}\n", module)
        self.assertIn("COPY mise.toml mise.lock mise.build.toml mise.build.lock", dockerfile)
        templ = shared["go:github.com/a-h/templ/cmd/templ"]
        goose = dev["go:github.com/pressly/goose/v3/cmd/goose"]
        self.assertIn(f"github.com/a-h/templ {templ}", module)
        self.assertIn(f"github.com/pressly/goose/v3 {goose}", module)
        self.assertIn("mise install --locked", dockerfile)
        self.assertIn(f"templ/cmd/templ@{templ}", workflow)
        tailwind = shared["github:tailwindlabs/tailwindcss"]
        self.assertNotIn("ARG TAILWIND_VERSION", dockerfile)
        self.assertIn(f"releases/download/{tailwind}/tailwindcss-linux-x64", workflow)

    def test_lockfiles_match_their_owning_configuration(self):
        for environment in ("", ".development", ".build"):
            with self.subTest(environment=environment or "shared"):
                tools = config(f"mise{environment}.toml")["tools"]
                locked = config(f"mise{environment}.lock")["tools"]
                self.assertEqual(set(locked), set(tools))
                for name, version in tools.items():
                    self.assertEqual(len(locked[name]), 1, name)
                    self.assertEqual(locked[name][0]["version"], version.removeprefix("v"))
                    self.assertEqual(locked[name][0]["specifiers"], [version])

    def test_locked_tailwind_artifact_matches_existing_pipeline(self):
        tool = config("mise.lock")["tools"]["github:tailwindlabs/tailwindcss"][0]
        artifact = tool["platforms.linux-x64"]
        dockerfile = (ROOT / "Dockerfile").read_text()
        workflow = (ROOT / ".github/workflows/container.yml").read_text()
        self.assertIn(artifact["url"], workflow)
        self.assertTrue(artifact["checksum"].startswith("sha256:"))
        digest = artifact["checksum"].removeprefix("sha256:")
        self.assertIn("COPY mise.toml mise.lock mise.build.toml mise.build.lock", dockerfile)
        self.assertIn(digest, workflow)

    def test_personal_overrides_are_excluded_from_git_and_docker(self):
        for name in (".gitignore", ".dockerignore"):
            with self.subTest(name=name):
                self.assertIn("**/*.local.toml", (ROOT / name).read_text().splitlines())


if __name__ == "__main__":
    unittest.main()
