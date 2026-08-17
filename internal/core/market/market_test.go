package market

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func setup(t *testing.T) (*library.Library, *portstest.MemFS) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/src/my-plugin/.claude-plugin", 0o755))
	must(m.WriteFile("/src/my-plugin/.claude-plugin/plugin.json", []byte(`{"name":"my-plugin"}`), 0o644))
	must(m.MkdirAll("/src/my-skill", 0o755))
	must(m.WriteFile("/src/my-skill/SKILL.md", []byte("skill"), 0o644))
	clock := &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}
	lib := library.New(m, "/home/.nexo", clock)
	if _, err := lib.Add("/src/my-plugin", domain.ID{Source: "local", Name: "my-plugin"}, library.Sidecar{Type: "plugin", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Add("/src/my-skill", domain.ID{Source: "local", Name: "my-skill"}, library.Sidecar{}); err != nil {
		t.Fatal(err)
	}
	return lib, m
}

func sync(t *testing.T, lib *library.Library, m *portstest.MemFS) []string {
	t.Helper()
	steps, names, err := SyncSteps(m, lib, "/home/.nexo", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&tx.Engine{FS: m}).Run("sync", steps); err != nil {
		t.Fatal(err)
	}
	return names
}

func TestSyncExposesOnlyPlugins(t *testing.T) {
	lib, m := setup(t)
	names := sync(t, lib, m)
	if len(names) != 1 || names[0] != "my-plugin" {
		t.Fatalf("names = %v", names)
	}
	// Plugin symlinked into the marketplace.
	target, err := m.Readlink("/home/.nexo/marketplace/my-plugin")
	if err != nil || target != "/home/.nexo/library/local/my-plugin" {
		t.Errorf("link = %q, %v", target, err)
	}
	// Skill NOT exposed.
	if _, err := m.Lstat("/home/.nexo/marketplace/my-skill"); err == nil {
		t.Error("skill leaked into the marketplace")
	}
	// marketplace.json valid and complete.
	data, err := m.ReadFile("/home/.nexo/marketplace/.claude-plugin/marketplace.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest marketplaceJSON
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("invalid marketplace.json: %v", err)
	}
	if manifest.Name != Name || len(manifest.Plugins) != 1 || manifest.Plugins[0].Source != "./my-plugin" {
		t.Errorf("manifest = %+v", manifest)
	}
}

func TestSyncIsIdempotentAndDropsStale(t *testing.T) {
	lib, m := setup(t)
	sync(t, lib, m)
	// Second sync: no link churn needed, still correct.
	sync(t, lib, m)

	// Remove the plugin from the library → next sync drops the link.
	if err := lib.Remove(domain.ID{Source: "local", Name: "my-plugin"}); err != nil {
		t.Fatal(err)
	}
	names := sync(t, lib, m)
	if len(names) != 0 {
		t.Fatalf("names after removal = %v", names)
	}
	if _, err := m.Lstat("/home/.nexo/marketplace/my-plugin"); err == nil {
		t.Error("stale link survived sync")
	}
	data, _ := m.ReadFile("/home/.nexo/marketplace/.claude-plugin/marketplace.json")
	if strings.Contains(string(data), "my-plugin") {
		t.Error("removed plugin still listed in marketplace.json")
	}
}
