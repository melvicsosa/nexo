// Package providers defines the contract every provider adapter
// implements and the shared inspection helpers. The core never
// contains provider-specific logic (spec §3): adapters translate
// between nexo's model and each tool's native layout, and they declare
// capabilities explicitly — the UI queries, never assumes (spec §23).
package providers

import (
	"errors"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/core/treehash"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
)

// DetectionResult says whether the provider looks present on this
// machine and what evidence supports that.
type DetectionResult struct {
	Installed bool   `json:"installed"`
	Evidence  string `json:"evidence,omitempty"`
}

// Capabilities declares what a provider supports. Absent capability =
// the UI hides or disables the action; it never guesses.
type Capabilities struct {
	GlobalSkills  bool `json:"global_skills"`
	ProjectSkills bool `json:"project_skills"`
	Plugins       bool `json:"plugins"`
	MCP           bool `json:"mcp"`
}

// FoundAsset is one asset discovered by inspection. Hash carries the
// content identity so later phases can classify Managed vs Detected vs
// Broken; inspection itself stays read-only.
type FoundAsset struct {
	Name    string      `json:"name"`
	Type    domain.Type `json:"type"`
	Path    string      `json:"path"`
	Hash    string      `json:"hash,omitempty"`
	Version string      `json:"version,omitempty"`
	Enabled *bool       `json:"enabled,omitempty"` // plugins: enable state when known
	Origin  string      `json:"origin"`            // which provider mechanism reported it
}

// Provider is the read side of the adapter contract (Phase 2). The
// write side — Plan(asset, target) consumed by the tx engine — lands in
// Phase 3 as a separate interface so read-only adapters stay honest.
type Provider interface {
	ID() string
	Name() string
	Detect() DetectionResult
	Capabilities() Capabilities
	InspectGlobal() ([]FoundAsset, error)
	InspectProject(projectPath string) ([]FoundAsset, error)
}

// ScanSkillsDir inspects a skills directory the way the tools do: every
// child directory containing a SKILL.md is a skill. Symlinked children
// are followed — real projects symlink skill dirs to shared locations —
// and a missing directory reports zero assets, not an error.
func ScanSkillsDir(fsys ports.FS, dir, origin string) ([]FoundAsset, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []FoundAsset
	for _, e := range entries {
		child := path.Join(dir, e.Name())
		info, err := fsys.Stat(child) // follows symlinks
		if err != nil || !info.IsDir() {
			continue // dangling link or plain file: not a skill dir
		}
		if _, err := fsys.Stat(path.Join(child, "SKILL.md")); err != nil {
			continue
		}
		hash, err := treehash.Tree(fsys, child)
		if err != nil {
			return nil, err
		}
		out = append(out, FoundAsset{
			Name:   e.Name(),
			Type:   domain.TypeSkill,
			Path:   child,
			Hash:   hash,
			Origin: origin,
		})
	}
	return out, nil
}
