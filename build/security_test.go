package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the same fail-closed package floor used in independent image bases, without
// downloading certificates or contacting any live service.
func TestCertificatePackageFloor(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		queryFails    bool
		wantOK        bool
	}{
		{name: "stale", version: "20230311+deb12u1"},
		{name: "previous update", version: "20250419~deb12u1"},
		{name: "floor", version: "20250419~deb12u2", wantOK: true},
		{name: "newer", version: "20250419~deb12u3", wantOK: true},
		{name: "missing package", queryFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			query := "#!/bin/sh\nprintf '%s' '" + tc.version + "'\n"
			if tc.queryFails {
				query = "#!/bin/sh\nexit 1\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "dpkg-query"), []byte(query), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", "check-ca-certificates.sh")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.wantOK {
				t.Fatalf("floor validation success=%v, want %v: %s", err == nil, tc.wantOK, out)
			}
			if !tc.wantOK && !tc.queryFails && !strings.Contains(string(out), "below required") {
				t.Fatalf("stale package failed for the wrong reason: %s", out)
			}
		})
	}
}

func TestImageTrustConfiguration(t *testing.T) {
	data, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	for path, pin := range map[string]string{"../go.mod": "go 1.27.4", "../mise.toml": "go = \"1.27.4\""} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), pin) {
			t.Errorf("%s must pin patched Go: %s", path, pin)
		}
	}
	// Only independent Bookworm bases install/check the bundle. Derived targets
	// inherit the protected finance-base rather than repeating the installation.
	stages := map[string]string{}
	for _, stage := range strings.Split(dockerfile, "\nFROM ")[1:] {
		header := strings.Fields(strings.SplitN(stage, "\n", 2)[0])
		stages[header[len(header)-1]] = stage
	}
	for _, name := range []string{"build", "finance-base", "backup"} {
		stage, ok := stages[name]
		if !ok {
			t.Fatalf("missing protected base stage %q", name)
		}
		for _, required := range []string{
			"COPY build/check-ca-certificates.sh /usr/local/bin/check-ca-certificates",
			"rm -rf /var/lib/apt/lists/*",
			"apt-get update",
			"sh /usr/local/bin/check-ca-certificates",
		} {
			if !strings.Contains(stage, required) {
				t.Errorf("%s stage must contain %q", name, required)
			}
		}
	}
	for _, derived := range []string{"categorizer", "runtime"} {
		if !strings.Contains(dockerfile, "FROM finance-base AS "+derived) {
			t.Errorf("%s must inherit the protected finance-base", derived)
		}
	}
	workflow, err := os.ReadFile("../.github/workflows/container.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"docker build --pull", "pull: true"} {
		if !strings.Contains(string(workflow), required) {
			t.Errorf("CI must refresh bases with %q", required)
		}
	}
}
