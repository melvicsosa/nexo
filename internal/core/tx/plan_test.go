package tx

import (
	"errors"
	"maps"
	"testing"

	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func seedTree(t *testing.T) *portstest.MemFS {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/lib/asset/ref", 0o755))
	must(m.WriteFile("/lib/asset/SKILL.md", []byte("skill"), 0o644))
	must(m.WriteFile("/lib/asset/ref/notes.md", []byte("notes"), 0o644))
	must(m.WriteFile("/lib/asset/.nexo.json", []byte("{}"), 0o644))
	must(m.Symlink("../shared", "/lib/asset/link"))
	must(m.MkdirAll("/dst", 0o755))
	return m
}

func TestPlanCopyTreeRoundTrip(t *testing.T) {
	m := seedTree(t)
	steps, err := PlanCopyTree(m, "/lib/asset", "/dst/asset", map[string]bool{".nexo.json": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Engine{FS: m}).Run("copy", steps); err != nil {
		t.Fatal(err)
	}
	if data, err := m.ReadFile("/dst/asset/SKILL.md"); err != nil || string(data) != "skill" {
		t.Errorf("SKILL.md = %q, %v", data, err)
	}
	if data, err := m.ReadFile("/dst/asset/ref/notes.md"); err != nil || string(data) != "notes" {
		t.Errorf("notes.md = %q, %v", data, err)
	}
	if target, err := m.Readlink("/dst/asset/link"); err != nil || target != "../shared" {
		t.Errorf("symlink = %q, %v", target, err)
	}
	// Ignored sidecar must not travel with the copy.
	if _, err := m.Lstat("/dst/asset/.nexo.json"); err == nil {
		t.Error("ignored file was copied")
	}
}

func TestPlanCopyTreeRollsBack(t *testing.T) {
	m := seedTree(t)
	before := m.Snapshot()
	boom := errors.New("boom")
	fault := &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"rename /dst/asset/ref/notes.md" + tmpSuffix: boom,
	}}
	steps, err := PlanCopyTree(m, "/lib/asset", "/dst/asset", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Engine{FS: fault}).Run("copy", steps); !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("copy failure left residue:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

func TestPlanRemoveTree(t *testing.T) {
	m := seedTree(t)
	steps, err := PlanRemoveTree(m, "/lib/asset")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Engine{FS: m}).Run("remove", steps); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lstat("/lib/asset"); err == nil {
		t.Error("tree still present after removal")
	}
}

func TestPlanRemoveTreeRollsBack(t *testing.T) {
	m := seedTree(t)
	before := m.Snapshot()
	boom := errors.New("boom")
	// Fail near the end: removing the root dir itself.
	fault := &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"remove /lib/asset": boom,
	}}
	steps, err := PlanRemoveTree(m, "/lib/asset")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Engine{FS: fault}).Run("remove", steps); !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("remove failure not fully rolled back:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

func TestPlanRemoveTreeSymlinkRootRemovesOnlyLink(t *testing.T) {
	m := seedTree(t)
	if err := m.Symlink("/lib/asset", "/dst/linked"); err != nil {
		t.Fatal(err)
	}
	steps, err := PlanRemoveTree(m, "/dst/linked")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatalf("symlink root plan = %d steps, want 1", len(steps))
	}
	if err := (&Engine{FS: m}).Run("unlink", steps); err != nil {
		t.Fatal(err)
	}
	// The target must be untouched.
	if _, err := m.ReadFile("/lib/asset/SKILL.md"); err != nil {
		t.Error("removing symlink root traversed into the target")
	}
}
