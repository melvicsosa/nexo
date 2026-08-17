package config

import (
	"encoding/json"
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func TestOpenInitializesFreshStore(t *testing.T) {
	m := portstest.NewMemFS()
	s, err := Open(m, "/home/.nexo")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.State().SchemaVersion != SchemaVersion {
		t.Errorf("schema = %d, want %d", s.State().SchemaVersion, SchemaVersion)
	}
	// The version must be persisted, not just in memory.
	data, err := m.ReadFile("/home/.nexo/state.json")
	if err != nil {
		t.Fatalf("state.json not written: %v", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil || st.SchemaVersion != SchemaVersion {
		t.Errorf("persisted state = %+v, %v", st, err)
	}
}

func TestOpenReopensExistingStore(t *testing.T) {
	m := portstest.NewMemFS()
	if _, err := Open(m, "/home/.nexo"); err != nil {
		t.Fatal(err)
	}
	s, err := Open(m, "/home/.nexo")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s.State().SchemaVersion != SchemaVersion {
		t.Errorf("schema after reopen = %d", s.State().SchemaVersion)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	m := portstest.NewMemFS()
	if err := m.MkdirAll("/home/.nexo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/home/.nexo/state.json", []byte(`{"schema_version": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(m, "/home/.nexo"); err == nil {
		t.Error("state from a newer nexo must be refused, not silently downgraded")
	}
}

func TestOpenRejectsCorruptState(t *testing.T) {
	m := portstest.NewMemFS()
	if err := m.MkdirAll("/home/.nexo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/home/.nexo/state.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(m, "/home/.nexo"); err == nil {
		t.Error("corrupt state.json must be an error")
	}
}

func TestOpenRejectsUnknownOlderSchema(t *testing.T) {
	// Schema 0 has no registered migration: Open must fail loudly
	// rather than guess at a layout it does not understand.
	m := portstest.NewMemFS()
	if err := m.MkdirAll("/home/.nexo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/home/.nexo/state.json", []byte(`{"schema_version": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(m, "/home/.nexo"); err == nil {
		t.Error("unmigratable schema must be an error")
	}
}
