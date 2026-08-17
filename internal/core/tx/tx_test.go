package tx

import (
	"errors"
	"io/fs"
	"maps"
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func seed(t *testing.T) *portstest.MemFS {
	t.Helper()
	m := portstest.NewMemFS()
	for _, d := range []string{"/home/.claude/skills", "/lib/asset"} {
		if err := m.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.WriteFile("/lib/asset/SKILL.md", []byte("skill content"), 0o644); err != nil {
		t.Fatal(err)
	}
	return m
}

func installPlan() []Step {
	return []Step{
		MkdirAll("/home/project/.claude/skills/asset", 0o755),
		WriteFile("/home/project/.claude/skills/asset/SKILL.md", []byte("skill content"), 0o644),
		Symlink("/lib/asset", "/home/.claude/skills/asset"),
	}
}

func TestRunSuccess(t *testing.T) {
	m := seed(t)
	e := &Engine{FS: m}
	if err := e.Run("install", installPlan()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if data, err := m.ReadFile("/home/project/.claude/skills/asset/SKILL.md"); err != nil || string(data) != "skill content" {
		t.Errorf("copied file = %q, %v", data, err)
	}
	if target, err := m.Readlink("/home/.claude/skills/asset"); err != nil || target != "/lib/asset" {
		t.Errorf("symlink = %q, %v", target, err)
	}
	// No staging leftovers.
	if _, err := m.Lstat("/home/project/.claude/skills/asset/SKILL.md" + tmpSuffix); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("staging leftover present: %v", err)
	}
}

func TestRunRollsBackOnFailure(t *testing.T) {
	// Fail each step in turn; whatever the failure point, the
	// filesystem must come back byte-identical to the initial state.
	targets := []struct {
		name string
		key  string
	}{
		{"mkdir fails", "mkdirall /home/project/.claude/skills/asset"},
		{"stage write fails", "writefile /home/project/.claude/skills/asset/SKILL.md" + tmpSuffix},
		{"commit rename fails", "rename /home/project/.claude/skills/asset/SKILL.md" + tmpSuffix},
		{"symlink fails", "symlink /home/.claude/skills/asset"},
	}
	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			m := seed(t)
			before := m.Snapshot()
			boom := errors.New("disk full")
			e := &Engine{FS: &portstest.FaultFS{Inner: m, FailOn: map[string]error{tt.key: boom}}}

			err := e.Run("install", installPlan())
			if !errors.Is(err, boom) {
				t.Fatalf("Run error = %v, want wrapped %v", err, boom)
			}
			if !maps.Equal(before, m.Snapshot()) {
				t.Errorf("filesystem not restored after rollback:\nbefore: %v\nafter:  %v", before, m.Snapshot())
			}
		})
	}
}

func TestWriteFileRestoresPriorContent(t *testing.T) {
	m := seed(t)
	if err := m.WriteFile("/home/.claude/skills/existing.md", []byte("PRECIOUS"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot()

	boom := errors.New("boom")
	e := &Engine{FS: &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"symlink /home/.claude/skills/asset": boom,
	}}}
	err := e.Run("overwrite-then-fail", []Step{
		WriteFile("/home/.claude/skills/existing.md", []byte("overwritten"), 0o644),
		Symlink("/lib/asset", "/home/.claude/skills/asset"),
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("prior content not restored:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

func TestStepGuards(t *testing.T) {
	tests := []struct {
		name string
		prep func(t *testing.T, m *portstest.MemFS)
		step Step
	}{
		{
			"write over directory refused",
			func(t *testing.T, m *portstest.MemFS) {
				if err := m.MkdirAll("/home/d", 0o755); err != nil {
					t.Fatal(err)
				}
			},
			WriteFile("/home/d", []byte("x"), 0o644),
		},
		{
			"write over symlink refused",
			func(t *testing.T, m *portstest.MemFS) {
				if err := m.Symlink("/lib/asset", "/home/link"); err != nil {
					t.Fatal(err)
				}
			},
			WriteFile("/home/link", []byte("x"), 0o644),
		},
		{
			"symlink over existing refused",
			func(t *testing.T, m *portstest.MemFS) {
				if err := m.WriteFile("/home/f", nil, 0o644); err != nil {
					t.Fatal(err)
				}
			},
			Symlink("/lib/asset", "/home/f"),
		},
		{
			"remove of directory refused",
			func(t *testing.T, m *portstest.MemFS) {
				if err := m.MkdirAll("/home/d", 0o755); err != nil {
					t.Fatal(err)
				}
			},
			Remove("/home/d"),
		},
		{
			"remove of missing path refused",
			func(t *testing.T, m *portstest.MemFS) {},
			Remove("/home/nope"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := seed(t)
			tt.prep(t, m)
			before := m.Snapshot()
			e := &Engine{FS: m}
			if err := e.Run("guarded", []Step{tt.step}); err == nil {
				t.Fatal("expected error, got nil")
			}
			if !maps.Equal(before, m.Snapshot()) {
				t.Error("guarded failure still mutated the filesystem")
			}
		})
	}
}

func TestRemoveUndoRestoresSymlinkAndFile(t *testing.T) {
	m := seed(t)
	if err := m.Symlink("/lib/asset", "/home/link"); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot()

	boom := errors.New("late failure")
	e := &Engine{FS: &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"symlink /home/.claude/skills/asset": boom,
	}}}
	err := e.Run("remove-then-fail", []Step{
		Remove("/home/link"),
		Remove("/lib/asset/SKILL.md"),
		Symlink("/lib/asset", "/home/.claude/skills/asset"),
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("removed entries not restored:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

func TestMkdirAllUndoRemovesOnlyCreated(t *testing.T) {
	m := seed(t)
	// /home/.claude exists; the plan creates skills2/deep and then fails.
	boom := errors.New("boom")
	e := &Engine{FS: &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"symlink /home/.claude/skills/asset": boom,
	}}}
	before := m.Snapshot()
	err := e.Run("mkdir-then-fail", []Step{
		MkdirAll("/home/.claude/skills2/deep", 0o755),
		Symlink("/lib/asset", "/home/.claude/skills/asset"),
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("created dirs not fully removed, or pre-existing dirs removed:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

func TestJournalLifecycle(t *testing.T) {
	m := portstest.NewMemFS()
	clock := &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}
	j := &FileJournal{FS: m, Dir: "/state/journal", Clock: clock}

	// Success: journal entry is created and removed.
	e := &Engine{FS: m, Journal: j}
	if err := m.MkdirAll("/dst", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.Run("ok", []Step{WriteFile("/dst/f", []byte("x"), 0o644)}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	pending, err := j.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("committed tx left journal entries: %v", pending)
	}

	// Failure: the record survives with status failed and the cause.
	boom := errors.New("boom")
	fe := &Engine{FS: &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"rename /dst/g" + tmpSuffix: boom,
	}}, Journal: j}
	if err := fe.Run("fails", []Step{WriteFile("/dst/g", []byte("y"), 0o644)}); !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	pending, err = j.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Status != "failed" || pending[0].Name != "fails" {
		t.Errorf("failed tx journal = %+v", pending)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	m := seed(t)
	before := m.Snapshot()
	descs := DryRun(installPlan())
	if len(descs) != 3 {
		t.Errorf("DryRun returned %d steps, want 3", len(descs))
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Error("DryRun mutated the filesystem")
	}
}

var _ ports.FS = (*portstest.MemFS)(nil)
