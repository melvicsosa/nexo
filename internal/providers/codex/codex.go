// Package codex is the Codex CLI provider adapter — skills only in v1.
// Research spike in docs/providers/codex.md, grounded in a real
// machine (2026-08-16): ~/.codex/skills/<name>/SKILL.md follows the
// same convention as Claude Code and Cursor. Plugin state lives in
// config.toml ([plugins."name@marketplace"] enabled = true) — mutating
// TOML safely needs a round-tripping parser, so the Plugins capability
// is declared false rather than half-supported (spec §23).
package codex

import (
	"fmt"
	"path"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
)

// Adapter implements providers.Provider for Codex.
type Adapter struct {
	fs    ports.FS
	paths ports.Paths
}

// New builds the adapter with its ports injected (plan D7).
func New(fsys ports.FS, paths ports.Paths) *Adapter {
	return &Adapter{fs: fsys, paths: paths}
}

func (a *Adapter) ID() string   { return "codex" }
func (a *Adapter) Name() string { return "Codex" }

func (a *Adapter) root() string { return path.Join(a.paths.Home(), ".codex") }

// Detect reports Codex as present when ~/.codex exists as a directory.
func (a *Adapter) Detect() providers.DetectionResult {
	info, err := a.fs.Stat(a.root())
	if err != nil || !info.IsDir() {
		return providers.DetectionResult{Installed: false}
	}
	return providers.DetectionResult{Installed: true, Evidence: a.root()}
}

// SkillsDir locates the skills directory for a target.
func (a *Adapter) SkillsDir(target domain.Target) string {
	if target.Scope == domain.ScopeProject {
		return path.Join(target.ProjectPath, ".codex", "skills")
	}
	return path.Join(a.root(), "skills")
}

func (a *Adapter) Capabilities() providers.Capabilities {
	return providers.Capabilities{
		GlobalSkills:  true,
		ProjectSkills: true,
		Plugins:       false, // TOML mutation deferred — see docs/providers/codex.md
		MCP:           false,
	}
}

// InspectGlobal reports skills from ~/.codex/skills.
func (a *Adapter) InspectGlobal() ([]providers.FoundAsset, error) {
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(a.root(), "skills"), "codex:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("codex: global skills: %w", err)
	}
	return skills, nil
}

// InspectProject reports skills from <project>/.codex/skills.
func (a *Adapter) InspectProject(projectPath string) ([]providers.FoundAsset, error) {
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(projectPath, ".codex", "skills"), "codex:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("codex: project skills: %w", err)
	}
	return skills, nil
}
