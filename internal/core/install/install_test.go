package install

import (
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
	"github.com/melvicsosa/nexo/internal/providers/claudecode"
)

func setup(t *testing.T) (*Installer, *portstest.MemFS, *claudecode.Adapter) {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.claude", 0o755))
	must(m.MkdirAll("/repo", 0o755))
	must(m.MkdirAll("/src/my-skill", 0o755))
	must(m.WriteFile("/src/my-skill/SKILL.md", []byte("v1"), 0o644))

	clock := &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}
	lib := library.New(m, "/home/.nexo", clock)
	if _, err := lib.Add("/src/my-skill", libID(), library.Sidecar{Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{
		FS:    m,
		Lib:   lib,
		DB:    OpenDB(m, "/home/.nexo"),
		Clock: clock,
	}
	prov := claudecode.New(m, portstest.FakePaths{HomeDir: "/home"})
	return inst, m, prov
}

func libID() domain.ID { return domain.ID{Source: "local", Name: "my-skill"} }

func globalReq(prov *claudecode.Adapter) Request {
	return Request{Asset: libID(), Provider: prov, Target: domain.Target{Scope: domain.ScopeGlobal}}
}

func projectReq(prov *claudecode.Adapter) Request {
	return Request{Asset: libID(), Provider: prov, Target: domain.Target{Scope: domain.ScopeProject, ProjectPath: "/repo"}}
}

func TestGlobalInstallIsSymlink(t *testing.T) {
	inst, m, prov := setup(t)
	if _, err := inst.Install(globalReq(prov)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	target, err := m.Readlink("/home/.claude/skills/my-skill")
	if err != nil || target != "/home/.nexo/library/local/my-skill" {
		t.Errorf("symlink = %q, %v (D3: global must symlink into the library)", target, err)
	}
	// Recorded.
	if _, found, _ := inst.DB.Find(libID(), "claude-code", domain.Target{Scope: domain.ScopeGlobal}); !found {
		t.Error("installation not recorded")
	}
	// Idempotent.
	res, err := inst.Install(globalReq(prov))
	if err != nil || !res.AlreadyInstalled {
		t.Errorf("re-install = %+v, %v, want AlreadyInstalled", res, err)
	}
}

func TestProjectInstallIsCopyWithManifest(t *testing.T) {
	inst, m, prov := setup(t)
	if _, err := inst.Install(projectReq(prov)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// A real copy, not a link (D3: repos must be self-contained).
	info, err := m.Lstat("/repo/.claude/skills/my-skill")
	if err != nil || !info.IsDir() {
		t.Fatalf("project install = %v, %v, want directory", info, err)
	}
	if data, _ := m.ReadFile("/repo/.claude/skills/my-skill/SKILL.md"); string(data) != "v1" {
		t.Errorf("content = %q", data)
	}
	// Sidecar must NOT be copied into the project.
	if _, err := m.Lstat("/repo/.claude/skills/my-skill/.nexo.json"); err == nil {
		t.Error("library sidecar leaked into the project")
	}
	// Manifest written and pinned.
	manifest, err := LoadManifest(m, "/repo")
	if err != nil || len(manifest.Assets) != 1 || manifest.Assets[0].ID != libID() {
		t.Errorf("manifest = %+v, %v", manifest, err)
	}
}

func TestProjectInstallRefusesDifferentContent(t *testing.T) {
	inst, m, prov := setup(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/repo/.claude/skills/my-skill", 0o755))
	must(m.WriteFile("/repo/.claude/skills/my-skill/SKILL.md", []byte("hand-edited"), 0o644))
	before := m.Snapshot()

	_, err := inst.Install(projectReq(prov))
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("install over different content = %v, want refusal mentioning --force (D5)", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Error("refused install still mutated the project")
	}

	// With --force it replaces.
	req := projectReq(prov)
	req.Force = true
	if _, err := inst.Install(req); err != nil {
		t.Fatalf("forced install: %v", err)
	}
	if data, _ := m.ReadFile("/repo/.claude/skills/my-skill/SKILL.md"); string(data) != "v1" {
		t.Errorf("forced content = %q", data)
	}
}

func TestProjectInstallIdenticalContentIsNoOp(t *testing.T) {
	inst, _, prov := setup(t)
	if _, err := inst.Install(projectReq(prov)); err != nil {
		t.Fatal(err)
	}
	res, err := inst.Install(projectReq(prov))
	if err != nil || !res.AlreadyInstalled {
		t.Errorf("identical re-install = %+v, %v", res, err)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	inst, m, prov := setup(t)
	before := m.Snapshot()
	req := projectReq(prov)
	req.DryRun = true
	res, err := inst.Install(req)
	if err != nil || !res.DryRun || len(res.Plan) == 0 {
		t.Fatalf("dry run = %+v, %v", res, err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Error("dry run mutated the filesystem")
	}
	if _, found, _ := inst.DB.Find(libID(), "claude-code", req.Target); found {
		t.Error("dry run recorded an installation")
	}
}

func TestRemoveManagedProject(t *testing.T) {
	inst, m, prov := setup(t)
	if _, err := inst.Install(projectReq(prov)); err != nil {
		t.Fatal(err)
	}
	res, err := inst.Remove(projectReq(prov))
	if err != nil || !res.Removed {
		t.Fatalf("Remove = %+v, %v", res, err)
	}
	if _, err := m.Lstat("/repo/.claude/skills/my-skill"); err == nil {
		t.Error("asset still present")
	}
	manifest, _ := LoadManifest(m, "/repo")
	if len(manifest.Assets) != 0 {
		t.Errorf("manifest entry survived: %+v", manifest.Assets)
	}
	if _, found, _ := inst.DB.Find(libID(), "claude-code", projectReq(prov).Target); found {
		t.Error("record survived")
	}
}

func TestRemoveRefusesModifiedContent(t *testing.T) {
	inst, m, prov := setup(t)
	if _, err := inst.Install(projectReq(prov)); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/repo/.claude/skills/my-skill/SKILL.md", []byte("edited after install"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := inst.Remove(projectReq(prov))
	if err == nil || !strings.Contains(err.Error(), "modified since") {
		t.Fatalf("remove modified = %v, want refusal (D6)", err)
	}
	req := projectReq(prov)
	req.Force = true
	if _, err := inst.Remove(req); err != nil {
		t.Fatalf("forced remove: %v", err)
	}
}

func TestRemoveRefusesUnmanaged(t *testing.T) {
	inst, m, prov := setup(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// A skill nexo never installed (Detected).
	must(m.MkdirAll("/repo/.claude/skills/my-skill", 0o755))
	must(m.WriteFile("/repo/.claude/skills/my-skill/SKILL.md", []byte("precious"), 0o644))

	_, err := inst.Remove(projectReq(prov))
	if err == nil || !strings.Contains(err.Error(), "not installed by nexo") {
		t.Fatalf("remove unmanaged = %v, want refusal (D6)", err)
	}
	// Still there.
	if _, err := m.ReadFile("/repo/.claude/skills/my-skill/SKILL.md"); err != nil {
		t.Error("refused removal still deleted the asset")
	}
}

func TestRemoveGlobalSymlink(t *testing.T) {
	inst, m, prov := setup(t)
	if _, err := inst.Install(globalReq(prov)); err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Remove(globalReq(prov)); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := m.Lstat("/home/.claude/skills/my-skill"); err == nil {
		t.Error("link still present")
	}
	// The library copy must be untouched.
	if _, err := m.ReadFile("/home/.nexo/library/local/my-skill/SKILL.md"); err != nil {
		t.Error("removing the install touched the library")
	}
}

func TestInstallRollsBackCompletely(t *testing.T) {
	inst, m, prov := setup(t)
	before := m.Snapshot()
	boom := errors.New("disk full")
	inst.FS = &portstest.FaultFS{Inner: m, FailOn: map[string]error{
		"rename /repo/.nexo/manifest.json.nexo-tmp": boom, // fail at the very last step
	}}
	_, err := inst.Install(projectReq(prov))
	if !errors.Is(err, boom) {
		t.Fatalf("Install error = %v", err)
	}
	if !maps.Equal(before, m.Snapshot()) {
		t.Errorf("failed install left residue:\nbefore: %v\nafter:  %v", before, m.Snapshot())
	}
}

var _ ports.FS = (*portstest.FaultFS)(nil)
