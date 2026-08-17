package claudecode

import (
	"testing"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

// fixture builds an in-memory home mirroring the real Claude Code
// layout verified on disk.
func fixture(t *testing.T) (*portstest.MemFS, portstest.FakePaths) {
	t.Helper()
	m := portstest.NewMemFS()
	paths := portstest.FakePaths{HomeDir: "/home"}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.claude/skills/go-testing", 0o755))
	must(m.WriteFile("/home/.claude/skills/go-testing/SKILL.md", []byte("---\nname: go-testing\n---\n"), 0o644))
	must(m.MkdirAll("/home/.claude/skills/not-a-skill", 0o755)) // no SKILL.md
	must(m.MkdirAll("/home/.claude/plugins", 0o755))
	must(m.WriteFile("/home/.claude/plugins/installed_plugins.json", []byte(`{
	  "version": 2,
	  "plugins": {
	    "engram@engram": [{"scope": "user", "installPath": "/home/.claude/plugins/cache/engram", "version": "0.1.0"}],
	    "vercel@official": [{"scope": "user", "installPath": "/home/.claude/plugins/cache/vercel", "version": "0.45.1"}]
	  }
	}`), 0o644))
	must(m.WriteFile("/home/.claude/settings.json", []byte(`{
	  "enabledPlugins": {"engram@engram": true, "ghost@nowhere": true}
	}`), 0o644))
	return m, paths
}

func TestDetect(t *testing.T) {
	m, paths := fixture(t)
	a := New(m, paths)
	if d := a.Detect(); !d.Installed || d.Evidence != "/home/.claude" {
		t.Errorf("Detect() = %+v", d)
	}
	empty := New(portstest.NewMemFS(), paths)
	if d := empty.Detect(); d.Installed {
		t.Errorf("Detect() on empty fs = %+v, want not installed", d)
	}
}

func TestInspectGlobal(t *testing.T) {
	m, paths := fixture(t)
	a := New(m, paths)
	assets, err := a.InspectGlobal()
	if err != nil {
		t.Fatalf("InspectGlobal: %v", err)
	}

	byName := map[string]int{}
	for i, asset := range assets {
		byName[asset.Name] = i
	}

	// Skill: found, hashed, dir without SKILL.md excluded.
	i, ok := byName["go-testing"]
	if !ok {
		t.Fatalf("go-testing skill not found in %+v", assets)
	}
	if assets[i].Type != domain.TypeSkill || assets[i].Hash == "" {
		t.Errorf("skill = %+v, want typed and hashed", assets[i])
	}
	if _, found := byName["not-a-skill"]; found {
		t.Error("dir without SKILL.md reported as skill")
	}

	// Plugin installed AND enabled.
	i, ok = byName["engram@engram"]
	if !ok || assets[i].Version != "0.1.0" || assets[i].Enabled == nil || !*assets[i].Enabled {
		t.Errorf("engram plugin = %+v, want version 0.1.0 enabled", assets[i])
	}
	// Plugin installed but NOT enabled.
	i, ok = byName["vercel@official"]
	if !ok || assets[i].Enabled == nil || *assets[i].Enabled {
		t.Errorf("vercel plugin = %+v, want disabled", assets[i])
	}
	// Enabled but not installed: the inconsistency must surface.
	i, ok = byName["ghost@nowhere"]
	if !ok || assets[i].Origin != "claude-code:settings.json" {
		t.Errorf("ghost plugin = %+v, want reported from settings.json", assets[i])
	}
}

func TestInspectGlobalMissingFilesAreNotErrors(t *testing.T) {
	m := portstest.NewMemFS()
	if err := m.MkdirAll("/home/.claude", 0o755); err != nil {
		t.Fatal(err)
	}
	a := New(m, portstest.FakePaths{HomeDir: "/home"})
	assets, err := a.InspectGlobal()
	if err != nil {
		t.Fatalf("InspectGlobal on bare install: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("bare install reported %+v", assets)
	}
}

func TestInspectGlobalCorruptJSON(t *testing.T) {
	m, paths := fixture(t)
	if err := m.WriteFile("/home/.claude/plugins/installed_plugins.json", []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(m, paths).InspectGlobal(); err == nil {
		t.Error("corrupt installed_plugins.json must error, not silently drop plugins")
	}
}

func TestInspectProject(t *testing.T) {
	m, paths := fixture(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Project skill via symlink to a shared dir — the real-world layout.
	must(m.MkdirAll("/repo/.agents/skills/api-auth", 0o755))
	must(m.WriteFile("/repo/.agents/skills/api-auth/SKILL.md", []byte("skill"), 0o644))
	must(m.MkdirAll("/repo/.claude/skills", 0o755))
	must(m.Symlink("../../.agents/skills/api-auth", "/repo/.claude/skills/api-auth"))
	must(m.WriteFile("/repo/.claude/settings.local.json", []byte(`{"enabledPlugins": {"team@corp": true}}`), 0o644))

	assets, err := New(m, paths).InspectProject("/repo")
	if err != nil {
		t.Fatalf("InspectProject: %v", err)
	}
	names := map[string]bool{}
	for _, asset := range assets {
		names[asset.Name] = true
	}
	if !names["api-auth"] {
		t.Errorf("symlinked project skill not found: %+v", assets)
	}
	if !names["team@corp"] {
		t.Errorf("project-enabled plugin not found: %+v", assets)
	}
}
