package library

import (
	"strings"
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func setup(t *testing.T) (*Library, *portstest.MemFS) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/src/my-skill/ref", 0o755))
	must(m.WriteFile("/src/my-skill/SKILL.md", []byte("# my skill"), 0o644))
	must(m.WriteFile("/src/my-skill/ref/notes.md", []byte("notes"), 0o644))
	must(m.WriteFile("/src/my-skill/.DS_Store", []byte("junk"), 0o644))
	clock := &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}
	return New(m, "/home/.nexo", clock), m
}

func id(name string) domain.ID { return domain.ID{Source: "local", Name: name} }

func TestAddGetList(t *testing.T) {
	lib, m := setup(t)
	asset, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{Version: "1.0.0", Description: "d"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if asset.Hash == "" || asset.Version != "1.0.0" || asset.Type != domain.TypeSkill {
		t.Errorf("asset = %+v", asset)
	}
	// Noise must not travel into the library.
	if _, err := m.Lstat("/home/.nexo/library/local/my-skill/.DS_Store"); err == nil {
		t.Error(".DS_Store copied into the library")
	}
	// Content must be there; sidecar too.
	if data, err := m.ReadFile("/home/.nexo/library/local/my-skill/SKILL.md"); err != nil || string(data) != "# my skill" {
		t.Errorf("content = %q, %v", data, err)
	}
	got, err := lib.Get(id("my-skill"))
	if err != nil || got.Hash != asset.Hash {
		t.Errorf("Get = %+v, %v", got, err)
	}
	assets, err := lib.List()
	if err != nil || len(assets) != 1 {
		t.Errorf("List = %+v, %v", assets, err)
	}
}

func TestAddIdempotentOnIdenticalContent(t *testing.T) {
	lib, _ := setup(t)
	if _, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{}); err != nil {
		t.Errorf("re-adding identical content must be a no-op, got %v", err)
	}
}

func TestAddRefusesDifferentContent(t *testing.T) {
	lib, m := setup(t)
	if _, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{}); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/src/my-skill/SKILL.md", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{})
	if err == nil || !strings.Contains(err.Error(), "different content") {
		t.Errorf("Add over different content = %v, want refusal", err)
	}
}

func TestSidecarExcludedFromHash(t *testing.T) {
	lib, m := setup(t)
	added, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	// The library copy (with sidecar inside) must hash identically to
	// the pristine source — metadata is not content.
	src := New(m, "/elsewhere", &portstest.FakeClock{})
	_ = src
	if got, _ := lib.Get(id("my-skill")); got.Hash != added.Hash {
		t.Errorf("hash changed after sidecar write: %s vs %s", got.Hash, added.Hash)
	}
}

func TestRemove(t *testing.T) {
	lib, m := setup(t)
	if _, err := lib.Add("/src/my-skill", id("my-skill"), Sidecar{}); err != nil {
		t.Fatal(err)
	}
	if err := lib.Remove(id("my-skill")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := m.Lstat("/home/.nexo/library/local/my-skill"); err == nil {
		t.Error("asset dir survived Remove")
	}
	if err := lib.Remove(id("my-skill")); err == nil {
		t.Error("removing a missing asset must error")
	}
}

func TestGetMissing(t *testing.T) {
	lib, _ := setup(t)
	if _, err := lib.Get(id("nope")); err == nil || !strings.Contains(err.Error(), "not in the library") {
		t.Errorf("Get missing = %v", err)
	}
}

func TestListEmptyLibrary(t *testing.T) {
	lib, _ := setup(t)
	assets, err := lib.List()
	if err != nil || assets != nil {
		t.Errorf("List on empty = %+v, %v", assets, err)
	}
}
