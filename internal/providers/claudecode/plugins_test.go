package claudecode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
)

func globalTarget() domain.Target { return domain.Target{Scope: domain.ScopeGlobal} }

func TestPlanPluginEnablePreservesUnknownFields(t *testing.T) {
	m, paths := fixture(t)
	// fixture settings.json has enabledPlugins; add unrelated fields.
	if err := m.WriteFile("/home/.claude/settings.json", []byte(`{
	  "model": "opus",
	  "permissions": {"deny": ["Bash(rm -rf /)"]},
	  "enabledPlugins": {"engram@engram": true}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(m, paths)
	steps, changed, err := a.PlanPluginEnable("vercel@official", globalTarget(), true)
	if err != nil || !changed {
		t.Fatalf("plan: changed=%v err=%v", changed, err)
	}
	if err := (&tx.Engine{FS: m}).Run("enable", steps); err != nil {
		t.Fatal(err)
	}
	data, err := m.ReadFile("/home/.claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	// The one key changed…
	enabled := settings["enabledPlugins"].(map[string]any)
	if enabled["vercel@official"] != true || enabled["engram@engram"] != true {
		t.Errorf("enabledPlugins = %v", enabled)
	}
	// …and everything nexo does not understand survived.
	if settings["model"] != "opus" {
		t.Errorf("model field lost: %v", settings["model"])
	}
	if _, ok := settings["permissions"].(map[string]any); !ok {
		t.Errorf("permissions field lost: %v", settings["permissions"])
	}
}

func TestPlanPluginEnableNoOpWhenAlreadySet(t *testing.T) {
	m, paths := fixture(t)
	a := New(m, paths)
	// engram is enabled in the fixture.
	_, changed, err := a.PlanPluginEnable("engram@engram", globalTarget(), true)
	if err != nil || changed {
		t.Errorf("enable already-enabled: changed=%v err=%v", changed, err)
	}
	// Absent plugin disabled: also a no-op.
	_, changed, err = a.PlanPluginEnable("never-heard@of-it", globalTarget(), false)
	if err != nil || changed {
		t.Errorf("disable absent: changed=%v err=%v", changed, err)
	}
}

func TestPlanPluginDisable(t *testing.T) {
	m, paths := fixture(t)
	a := New(m, paths)
	steps, changed, err := a.PlanPluginEnable("engram@engram", globalTarget(), false)
	if err != nil || !changed {
		t.Fatalf("plan: %v %v", changed, err)
	}
	if err := (&tx.Engine{FS: m}).Run("disable", steps); err != nil {
		t.Fatal(err)
	}
	enabled, err := readEnabledPlugins(m, "/home/.claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if enabled["engram@engram"] {
		t.Error("plugin still enabled after disable")
	}
}

func TestPlanPluginEnableCreatesMissingProjectSettings(t *testing.T) {
	m, paths := fixture(t)
	if err := m.MkdirAll("/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	a := New(m, paths)
	target := domain.Target{Scope: domain.ScopeProject, ProjectPath: "/repo"}
	steps, changed, err := a.PlanPluginEnable("team@corp", target, true)
	if err != nil || !changed {
		t.Fatalf("plan: %v %v", changed, err)
	}
	if err := (&tx.Engine{FS: m}).Run("enable", steps); err != nil {
		t.Fatal(err)
	}
	enabled, err := readEnabledPlugins(m, "/repo/.claude/settings.json")
	if err != nil || !enabled["team@corp"] {
		t.Errorf("project settings = %v, %v", enabled, err)
	}
}

func TestPlanPluginEnableRefusesCorruptSettings(t *testing.T) {
	m, paths := fixture(t)
	if err := m.WriteFile("/home/.claude/settings.json", []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := New(m, paths).PlanPluginEnable("x@y", globalTarget(), true)
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("corrupt settings = %v, want refusal", err)
	}
}
