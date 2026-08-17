package doctor

import (
	"testing"
	"time"

	"github.com/melvicsosa/nexo/internal/core/install"
	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
	"github.com/melvicsosa/nexo/internal/providers"
	"github.com/melvicsosa/nexo/internal/providers/claudecode"
)

func setup(t *testing.T) (Deps, *install.Installer, *portstest.MemFS, *claudecode.Adapter) {
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
	must(m.MkdirAll("/src/s", 0o755))
	must(m.WriteFile("/src/s/SKILL.md", []byte("x"), 0o644))

	clock := &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}
	lib := library.New(m, "/home/.nexo", clock)
	if _, err := lib.Add("/src/s", domain.ID{Source: "local", Name: "s"}, library.Sidecar{}); err != nil {
		t.Fatal(err)
	}
	db := install.OpenDB(m, "/home/.nexo")
	inst := &install.Installer{FS: m, Lib: lib, DB: db, Clock: clock}
	prov := claudecode.New(m, portstest.FakePaths{HomeDir: "/home"})
	deps := Deps{
		FS:       m,
		StateDir: "/home/.nexo",
		Lib:      lib,
		DB:       db,
		Provs:    map[string]providers.Provider{"claude-code": prov},
	}
	return deps, inst, m, prov
}

func req(prov *claudecode.Adapter, scope domain.Scope) install.Request {
	target := domain.Target{Scope: scope}
	if scope == domain.ScopeProject {
		target.ProjectPath = "/repo"
	}
	return install.Request{Asset: domain.ID{Source: "local", Name: "s"}, Provider: prov, Target: target}
}

func TestHealthySystemHasNoFindings(t *testing.T) {
	deps, inst, _, prov := setup(t)
	if _, err := inst.Install(req(prov, domain.ScopeProject)); err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Install(req(prov, domain.ScopeGlobal)); err != nil {
		t.Fatal(err)
	}
	findings, err := Run(deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("healthy system reported: %+v", findings)
	}
}

func TestDetectsMissingInstallation(t *testing.T) {
	deps, inst, m, prov := setup(t)
	if _, err := inst.Install(req(prov, domain.ScopeProject)); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveAll("/repo/.claude/skills/s"); err != nil {
		t.Fatal(err)
	}
	findings, err := Run(deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != "broken-missing" {
		t.Errorf("findings = %+v, want broken-missing", findings)
	}
}

func TestDetectsModifiedInstallation(t *testing.T) {
	deps, inst, m, prov := setup(t)
	if _, err := inst.Install(req(prov, domain.ScopeProject)); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/repo/.claude/skills/s/SKILL.md", []byte("drifted"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := Run(deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != "broken-modified" {
		t.Errorf("findings = %+v, want broken-modified", findings)
	}
}

func TestDetectsDanglingGlobalLink(t *testing.T) {
	deps, inst, _, prov := setup(t)
	if _, err := inst.Install(req(prov, domain.ScopeGlobal)); err != nil {
		t.Fatal(err)
	}
	// Removing the asset from the library orphans the global link —
	// exactly the trade-off Library.Remove documents.
	if err := deps.Lib.Remove(domain.ID{Source: "local", Name: "s"}); err != nil {
		t.Fatal(err)
	}
	findings, err := Run(deps)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range findings {
		if f.Code == "broken-dangling" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want broken-dangling", findings)
	}
}

func TestReportsUnknownProvider(t *testing.T) {
	deps, inst, _, prov := setup(t)
	if _, err := inst.Install(req(prov, domain.ScopeProject)); err != nil {
		t.Fatal(err)
	}
	deps.Provs = map[string]providers.Provider{} // provider vanished
	findings, err := Run(deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != "unknown-provider" {
		t.Errorf("findings = %+v", findings)
	}
}
