package registries

import (
	"strings"
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func setup(t *testing.T) (*Store, *portstest.MemFS) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/registry/assets/wordpress-review", 0o755))
	must(m.WriteFile("/registry/assets/wordpress-review/SKILL.md", []byte("wp"), 0o644))
	must(m.WriteFile("/registry/index.json", []byte(`{
	  "schema_version": 1,
	  "assets": [
	    {"name": "wordpress-review", "type": "skill", "version": "1.2.0",
	     "description": "Reviews WordPress PHP code", "path": "./assets/wordpress-review"}
	  ]
	}`), 0o644))
	return Open(m, "/home/.nexo"), m
}

func TestAddListRemove(t *testing.T) {
	store, _ := setup(t)
	if err := store.Add("company", "/registry"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].Name != "company" {
		t.Fatalf("List = %+v, %v", entries, err)
	}
	// Duplicates refused.
	if err := store.Add("company", "/registry"); err == nil {
		t.Error("duplicate registry accepted")
	}
	if err := store.Remove("company"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := store.Remove("company"); err == nil {
		t.Error("removing unknown registry must error")
	}
}

func TestAddValidatesUpfront(t *testing.T) {
	store, m := setup(t)
	tests := []struct {
		name string
		reg  string
		dir  string
	}{
		{"reserved local namespace", "local", "/registry"},
		{"invalid name", "has space", "/registry"},
		{"missing index", "ok", "/nowhere"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.Add(tt.reg, tt.dir); err == nil {
				t.Errorf("Add(%q, %q) accepted", tt.reg, tt.dir)
			}
		})
	}
	// Index escaping the registry dir is refused.
	if err := m.WriteFile("/registry/index.json", []byte(`{"schema_version":1,"assets":[{"name":"evil","path":"../../etc"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Add("evil", "/registry"); err == nil || !strings.Contains(err.Error(), "inside the registry") {
		t.Errorf("path traversal accepted: %v", err)
	}
}

func TestSearchAndResolve(t *testing.T) {
	store, _ := setup(t)
	if err := store.Add("company", "/registry"); err != nil {
		t.Fatal(err)
	}
	// Match by name fragment and by description, case-insensitive.
	for _, term := range []string{"wordpress", "PHP", "REVIEWS"} {
		hits, err := store.Search(term)
		if err != nil || len(hits) != 1 {
			t.Errorf("Search(%q) = %+v, %v", term, hits, err)
		}
	}
	hits, err := store.Search("nothing-like-this")
	if err != nil || len(hits) != 0 {
		t.Errorf("no-match search = %+v, %v", hits, err)
	}

	entry, dir, err := store.Resolve("company", "wordpress-review")
	if err != nil || entry.Version != "1.2.0" || dir != "/registry/assets/wordpress-review" {
		t.Errorf("Resolve = %+v, %q, %v", entry, dir, err)
	}
	if _, _, err := store.Resolve("company", "nope"); err == nil {
		t.Error("resolving unknown asset must error")
	}
	if _, _, err := store.Resolve("ghost", "x"); err == nil {
		t.Error("resolving unknown registry must error")
	}
}
