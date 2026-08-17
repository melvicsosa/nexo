package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

// newWriteApp builds an app on a machine where ONLY Claude Code is
// detected (so provider auto-selection is unambiguous) with one project
// skill ready to adopt.
func newWriteApp(t *testing.T) (*App, *portstest.MemFS, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.claude", 0o755))
	must(m.MkdirAll("/repo/.claude/skills/wordpress-review", 0o755))
	must(m.WriteFile("/repo/.claude/skills/wordpress-review/SKILL.md", []byte("# wp"), 0o644))

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := &App{
		FS:      m,
		Paths:   portstest.FakePaths{HomeDir: "/home"},
		Stdout:  stdout,
		Stderr:  stderr,
		Version: "test",
		Getwd:   func() (string, error) { return "/repo", nil },
		Clock:   &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
	}
	return app, m, stdout, stderr
}

func run(t *testing.T, app *App, stdout *bytes.Buffer, wantCode int, args ...string) string {
	t.Helper()
	stdout.Reset()
	if code := app.Run(args); code != wantCode {
		t.Fatalf("nexo %s: exit %d, want %d", strings.Join(args, " "), code, wantCode)
	}
	return stdout.String()
}

func TestAdoptInstallRemoveLifecycle(t *testing.T) {
	app, m, stdout, _ := newWriteApp(t)

	// Adopt by name: found in the current project's inspection.
	out := run(t, app, stdout, 0, "adopt", "wordpress-review")
	if !strings.Contains(out, "adopted local/wordpress-review") {
		t.Fatalf("adopt output: %s", out)
	}

	// Library shows it.
	out = run(t, app, stdout, 0, "library")
	if !strings.Contains(out, "local/wordpress-review") {
		t.Fatalf("library output: %s", out)
	}

	// Install globally: symlink into the library.
	out = run(t, app, stdout, 0, "install", "wordpress-review", "--global")
	if !strings.Contains(out, "installed local/wordpress-review") {
		t.Fatalf("install output: %s", out)
	}
	if target, err := m.Readlink("/home/.claude/skills/wordpress-review"); err != nil || !strings.Contains(target, "library") {
		t.Fatalf("global symlink = %q, %v", target, err)
	}

	// Doctor: healthy.
	out = run(t, app, stdout, 0, "doctor")
	if !strings.Contains(out, "everything checks out") {
		t.Fatalf("doctor output: %s", out)
	}

	// Remove it.
	run(t, app, stdout, 0, "remove", "wordpress-review", "--global")
	if _, err := m.Lstat("/home/.claude/skills/wordpress-review"); err == nil {
		t.Fatal("global link survived removal")
	}
}

func TestInstallIntoAnotherProject(t *testing.T) {
	app, m, stdout, _ := newWriteApp(t)
	if err := m.MkdirAll("/other", 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, app, stdout, 0, "adopt", "wordpress-review")
	run(t, app, stdout, 0, "install", "wordpress-review", "--project", "/other")

	if data, err := m.ReadFile("/other/.claude/skills/wordpress-review/SKILL.md"); err != nil || string(data) != "# wp" {
		t.Fatalf("installed copy = %q, %v", data, err)
	}
	// Manifest pinned in the target project.
	if data, err := m.ReadFile("/other/.nexo/manifest.json"); err != nil || !strings.Contains(string(data), "local/wordpress-review") {
		t.Fatalf("manifest = %q, %v", data, err)
	}
}

func TestInstallDryRun(t *testing.T) {
	app, m, stdout, _ := newWriteApp(t)
	run(t, app, stdout, 0, "adopt", "wordpress-review")
	before := m.Snapshot()
	out := run(t, app, stdout, 0, "install", "wordpress-review", "--global", "--dry-run")
	if !strings.Contains(out, "dry run") || !strings.Contains(out, "symlink") {
		t.Fatalf("dry run output: %s", out)
	}
	after := m.Snapshot()
	// State dir gets initialized, but nothing outside it may change.
	for k, v := range before {
		if !strings.HasPrefix(k, "/home/.nexo") && after[k] != v {
			t.Errorf("dry run changed %s", k)
		}
	}
}

func TestRemoveUnmanagedIsRefused(t *testing.T) {
	app, _, stdout, stderr := newWriteApp(t)
	run(t, app, stdout, 0, "adopt", "wordpress-review")
	// The project skill exists but was never installed BY NEXO there.
	stdout.Reset()
	if code := app.Run([]string{"remove", "wordpress-review", "--project", "/repo"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not installed by nexo") {
		t.Fatalf("stderr: %s", stderr.String())
	}
}

func TestDoctorFlagsDrift(t *testing.T) {
	app, m, stdout, _ := newWriteApp(t)
	run(t, app, stdout, 0, "adopt", "wordpress-review")
	if err := m.MkdirAll("/other", 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, app, stdout, 0, "install", "wordpress-review", "--project", "/other")
	// Drift the installed copy.
	if err := m.WriteFile("/other/.claude/skills/wordpress-review/SKILL.md", []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := run(t, app, stdout, 1, "doctor")
	if !strings.Contains(out, "broken-modified") {
		t.Fatalf("doctor output: %s", out)
	}
}

func TestAdoptUnknownName(t *testing.T) {
	app, _, stdout, stderr := newWriteApp(t)
	_ = stdout
	if code := app.Run([]string{"adopt", "does-not-exist"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "no skill named") {
		t.Fatalf("stderr: %s", stderr.String())
	}
}

func TestInstallMissingFromLibrary(t *testing.T) {
	app, _, _, stderr := newWriteApp(t)
	if code := app.Run([]string{"install", "never-adopted", "--global"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "not in the library") {
		t.Fatalf("stderr: %s", stderr.String())
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"flags after positional", []string{"asset", "--global", "--force"}, false},
		{"value flag with equals", []string{"--provider=cursor", "asset"}, false},
		{"value flag with space", []string{"--project", "/x", "asset"}, false},
		{"unknown flag", []string{"--wat"}, true},
		{"value flag without value", []string{"asset", "--provider"}, true},
		{"bool flag with value", []string{"--global=yes"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args, installBoolFlags, installValueFlags)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}
