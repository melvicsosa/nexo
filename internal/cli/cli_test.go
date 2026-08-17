package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func newApp(t *testing.T) (*App, *portstest.MemFS, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Claude Code present with one skill and one enabled plugin.
	must(m.MkdirAll("/home/.claude/skills/go-testing", 0o755))
	must(m.WriteFile("/home/.claude/skills/go-testing/SKILL.md", []byte("skill"), 0o644))
	must(m.MkdirAll("/home/.claude/plugins", 0o755))
	must(m.WriteFile("/home/.claude/plugins/installed_plugins.json",
		[]byte(`{"version":2,"plugins":{"engram@engram":[{"scope":"user","installPath":"/x","version":"0.1.0"}]}}`), 0o644))
	must(m.WriteFile("/home/.claude/settings.json", []byte(`{"enabledPlugins":{"engram@engram":true}}`), 0o644))
	// Cursor NOT installed on this fake machine.
	// A project with one claude skill.
	must(m.MkdirAll("/repo/.claude/skills/api-auth", 0o755))
	must(m.WriteFile("/repo/.claude/skills/api-auth/SKILL.md", []byte("skill"), 0o644))

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := &App{
		FS:      m,
		Paths:   portstest.FakePaths{HomeDir: "/home"},
		Stdout:  stdout,
		Stderr:  stderr,
		Version: "test",
		Getwd:   func() (string, error) { return "/repo", nil },
	}
	return app, m, stdout, stderr
}

func TestVersion(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"version"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := stdout.String(); got != "nexo test\n" {
		t.Errorf("version output = %q", got)
	}
}

func TestUnknownCommand(t *testing.T) {
	app, _, _, stderr := newApp(t)
	if code := app.Run([]string{"frobnicate"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestProviders(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"providers"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Claude Code") || !strings.Contains(out, "Cursor") {
		t.Errorf("providers output missing rows:\n%s", out)
	}
}

func TestProvidersJSON(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"providers", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var reports []providerReport
	if err := json.Unmarshal(stdout.Bytes(), &reports); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if len(reports) != 2 {
		t.Fatalf("got %d providers", len(reports))
	}
	if !reports[0].Detection.Installed || reports[1].Detection.Installed {
		t.Errorf("detection = %+v", reports)
	}
}

func TestList(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"list"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	out := stdout.String()
	for _, want := range []string{"go-testing", "engram@engram", "enabled 0.1.0", "cursor: not detected"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestListJSON(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"list", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var reports []globalReport
	if err := json.Unmarshal(stdout.Bytes(), &reports); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(reports) != 2 || len(reports[0].Assets) != 2 {
		t.Errorf("reports = %+v", reports)
	}
}

func TestProjectInspectDefaultsToCwd(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"project", "inspect"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Project: /repo") || !strings.Contains(out, "api-auth") {
		t.Errorf("inspect output:\n%s", out)
	}
}

func TestProjectInspectExplicitPathJSON(t *testing.T) {
	app, _, stdout, _ := newApp(t)
	if code := app.Run([]string{"project", "inspect", "/repo", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var report inspectReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if report.Project != "/repo" {
		t.Errorf("project = %q", report.Project)
	}
	found := false
	for _, p := range report.Providers {
		for _, asset := range p.Assets {
			if asset.Name == "api-auth" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("api-auth not in report: %+v", report)
	}
}

func TestProjectRequiresInspect(t *testing.T) {
	app, _, _, _ := newApp(t)
	if code := app.Run([]string{"project"}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestCorruptSettingsSurfacesError(t *testing.T) {
	app, m, _, stderr := newApp(t)
	if err := m.WriteFile("/home/.claude/settings.json", []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := app.Run([]string{"list"}); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "claude-code") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
