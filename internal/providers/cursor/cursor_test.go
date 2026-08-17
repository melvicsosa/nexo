package cursor

import (
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func TestDetectAndInspect(t *testing.T) {
	m := portstest.NewMemFS()
	paths := portstest.FakePaths{HomeDir: "/home"}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.cursor/skills/branch-pr", 0o755))
	must(m.WriteFile("/home/.cursor/skills/branch-pr/SKILL.md", []byte("skill"), 0o644))
	// Cursor's own synced set must NOT be reported.
	must(m.MkdirAll("/home/.cursor/skills-cursor/review", 0o755))
	must(m.WriteFile("/home/.cursor/skills-cursor/review/SKILL.md", []byte("cursor-owned"), 0o644))

	a := New(m, paths)
	if d := a.Detect(); !d.Installed {
		t.Errorf("Detect() = %+v", d)
	}
	assets, err := a.InspectGlobal()
	if err != nil {
		t.Fatalf("InspectGlobal: %v", err)
	}
	if len(assets) != 1 || assets[0].Name != "branch-pr" {
		t.Errorf("assets = %+v, want only branch-pr", assets)
	}

	// Not installed on a bare machine.
	if d := New(portstest.NewMemFS(), paths).Detect(); d.Installed {
		t.Errorf("bare Detect() = %+v", d)
	}
}

func TestInspectProject(t *testing.T) {
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.cursor", 0o755))
	must(m.MkdirAll("/repo/.cursor/skills/deploy", 0o755))
	must(m.WriteFile("/repo/.cursor/skills/deploy/SKILL.md", []byte("x"), 0o644))

	a := New(m, portstest.FakePaths{HomeDir: "/home"})
	assets, err := a.InspectProject("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Name != "deploy" {
		t.Errorf("assets = %+v", assets)
	}
	// Project without .cursor: zero assets, no error.
	assets, err = a.InspectProject("/other")
	if err != nil || len(assets) != 0 {
		t.Errorf("bare project = %+v, %v", assets, err)
	}
}
