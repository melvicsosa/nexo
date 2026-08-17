package codex

import (
	"testing"

	"github.com/melvicsosa/nexo/internal/domain"
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
	must(m.MkdirAll("/home/.codex/skills/go-testing", 0o755))
	must(m.WriteFile("/home/.codex/skills/go-testing/SKILL.md", []byte("skill"), 0o644))

	a := New(m, paths)
	if d := a.Detect(); !d.Installed || d.Evidence != "/home/.codex" {
		t.Errorf("Detect() = %+v", d)
	}
	assets, err := a.InspectGlobal()
	if err != nil || len(assets) != 1 || assets[0].Name != "go-testing" {
		t.Errorf("InspectGlobal = %+v, %v", assets, err)
	}
	if d := New(portstest.NewMemFS(), paths).Detect(); d.Installed {
		t.Errorf("bare Detect() = %+v", d)
	}
}

func TestSkillsDirAndProject(t *testing.T) {
	m := portstest.NewMemFS()
	a := New(m, portstest.FakePaths{HomeDir: "/home"})
	if got := a.SkillsDir(domain.Target{Scope: domain.ScopeGlobal}); got != "/home/.codex/skills" {
		t.Errorf("global SkillsDir = %q", got)
	}
	if got := a.SkillsDir(domain.Target{Scope: domain.ScopeProject, ProjectPath: "/repo"}); got != "/repo/.codex/skills" {
		t.Errorf("project SkillsDir = %q", got)
	}
	// Missing project dir: zero assets, no error.
	assets, err := a.InspectProject("/repo")
	if err != nil || len(assets) != 0 {
		t.Errorf("bare project = %+v, %v", assets, err)
	}
}
