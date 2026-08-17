// Package cursor is the Cursor provider adapter — read-only in v1,
// scoped to skills. The research spike behind these choices is
// documented in docs/providers/cursor.md: Cursor has its own skill
// sync mechanism (~/.cursor/skills-cursor + .sync-manifest.json) that
// nexo deliberately does not touch, and its plugin surface is not yet
// understood well enough to manage safely — so Plugins is declared
// unsupported instead of guessed at (spec §23).
package cursor

import (
	"fmt"
	"path"

	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
)

// Adapter implements providers.Provider for Cursor.
type Adapter struct {
	fs    ports.FS
	paths ports.Paths
}

// New builds the adapter with its ports injected (plan D7).
func New(fsys ports.FS, paths ports.Paths) *Adapter {
	return &Adapter{fs: fsys, paths: paths}
}

func (a *Adapter) ID() string   { return "cursor" }
func (a *Adapter) Name() string { return "Cursor" }

func (a *Adapter) root() string { return path.Join(a.paths.Home(), ".cursor") }

// Detect reports Cursor as present when ~/.cursor exists as a
// directory.
func (a *Adapter) Detect() providers.DetectionResult {
	info, err := a.fs.Stat(a.root())
	if err != nil || !info.IsDir() {
		return providers.DetectionResult{Installed: false}
	}
	return providers.DetectionResult{Installed: true, Evidence: a.root()}
}

func (a *Adapter) Capabilities() providers.Capabilities {
	return providers.Capabilities{
		GlobalSkills:  true,
		ProjectSkills: true,
		Plugins:       false, // see docs/providers/cursor.md
		MCP:           false,
	}
}

// InspectGlobal reports skills from ~/.cursor/skills. The adjacent
// skills-cursor directory is Cursor's own synced set and is
// intentionally not reported: nexo will never manage it.
func (a *Adapter) InspectGlobal() ([]providers.FoundAsset, error) {
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(a.root(), "skills"), "cursor:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("cursor: global skills: %w", err)
	}
	return skills, nil
}

// InspectProject reports skills from <project>/.cursor/skills.
func (a *Adapter) InspectProject(projectPath string) ([]providers.FoundAsset, error) {
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(projectPath, ".cursor", "skills"), "cursor:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("cursor: project skills: %w", err)
	}
	return skills, nil
}
