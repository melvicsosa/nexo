// Package claudecode is the Claude Code provider adapter. Everything
// here is grounded in the real on-disk layout (verified 2026-08-16):
//
//	~/.claude/skills/<name>/SKILL.md              global skills
//	~/.claude/plugins/installed_plugins.json      installed plugins (v2)
//	~/.claude/settings.json  → enabledPlugins     plugin enable state
//	<project>/.claude/skills/<name>/SKILL.md      project skills
//	<project>/.claude/settings.json + settings.local.json
//
// Plugins are NOT file copies (plan D1): they are references — a
// marketplace clone in a cache plus an enabledPlugins entry — so this
// adapter reports them from configuration, not from a directory scan.
package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
)

// Adapter implements providers.Provider for Claude Code.
type Adapter struct {
	fs    ports.FS
	paths ports.Paths
}

// New builds the adapter with its ports injected (plan D7).
func New(fsys ports.FS, paths ports.Paths) *Adapter {
	return &Adapter{fs: fsys, paths: paths}
}

func (a *Adapter) ID() string   { return "claude-code" }
func (a *Adapter) Name() string { return "Claude Code" }

func (a *Adapter) root() string { return path.Join(a.paths.Home(), ".claude") }

// Detect reports Claude Code as present when ~/.claude exists as a
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
		Plugins:       true,
		MCP:           false, // modeled (plan D12), not implemented in v1
	}
}

// InspectGlobal reports global skills from the skills dir and plugins
// from installed_plugins.json merged with settings.json enable state.
func (a *Adapter) InspectGlobal() ([]providers.FoundAsset, error) {
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(a.root(), "skills"), "claude-code:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("claude-code: global skills: %w", err)
	}
	plugins, err := a.inspectPlugins()
	if err != nil {
		return nil, err
	}
	return append(skills, plugins...), nil
}

// InspectProject reports project skills and any project-level plugin
// enablement from .claude/settings.json and .claude/settings.local.json.
func (a *Adapter) InspectProject(projectPath string) ([]providers.FoundAsset, error) {
	dir := path.Join(projectPath, ".claude")
	skills, err := providers.ScanSkillsDir(a.fs, path.Join(dir, "skills"), "claude-code:skills-dir")
	if err != nil {
		return nil, fmt.Errorf("claude-code: project skills: %w", err)
	}
	var out []providers.FoundAsset
	out = append(out, skills...)
	for _, file := range []string{"settings.json", "settings.local.json"} {
		p := path.Join(dir, file)
		enabled, err := readEnabledPlugins(a.fs, p)
		if err != nil {
			return nil, fmt.Errorf("claude-code: %s: %w", file, err)
		}
		for _, name := range sortedKeys(enabled) {
			state := enabled[name]
			out = append(out, providers.FoundAsset{
				Name:    name,
				Type:    domain.TypePlugin,
				Path:    p,
				Enabled: &state,
				Origin:  "claude-code:" + file,
			})
		}
	}
	return out, nil
}

// installedPluginsFile mirrors ~/.claude/plugins/installed_plugins.json
// (format version 2, verified in the wild).
type installedPluginsFile struct {
	Version int `json:"version"`
	Plugins map[string][]struct {
		Scope       string `json:"scope"`
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
	} `json:"plugins"`
}

func (a *Adapter) inspectPlugins() ([]providers.FoundAsset, error) {
	installedPath := path.Join(a.root(), "plugins", "installed_plugins.json")
	enabled, err := readEnabledPlugins(a.fs, path.Join(a.root(), "settings.json"))
	if err != nil {
		return nil, fmt.Errorf("claude-code: settings.json: %w", err)
	}

	data, err := a.fs.ReadFile(installedPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude-code: installed_plugins.json: %w", err)
	}
	var file installedPluginsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("claude-code: installed_plugins.json: %w", err)
	}

	var out []providers.FoundAsset
	for _, name := range sortedKeys(file.Plugins) {
		entries := file.Plugins[name]
		version, installPath := "", installedPath
		if len(entries) > 0 {
			version, installPath = entries[0].Version, entries[0].InstallPath
		}
		state := enabled[name]
		out = append(out, providers.FoundAsset{
			Name:    name,
			Type:    domain.TypePlugin,
			Path:    installPath,
			Version: version,
			Enabled: &state,
			Origin:  "claude-code:installed_plugins",
		})
		delete(enabled, name)
	}
	// Enabled but not installed: report it — that inconsistency is
	// exactly what inspection exists to surface.
	for _, name := range sortedKeys(enabled) {
		state := enabled[name]
		out = append(out, providers.FoundAsset{
			Name:    name,
			Type:    domain.TypePlugin,
			Path:    path.Join(a.root(), "settings.json"),
			Enabled: &state,
			Origin:  "claude-code:settings.json",
		})
	}
	return out, nil
}

// readEnabledPlugins extracts the enabledPlugins map from a settings
// file; a missing file means no opinions, not an error.
func readEnabledPlugins(fsys ports.FS, p string) (map[string]bool, error) {
	data, err := fsys.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	if settings.EnabledPlugins == nil {
		return map[string]bool{}, nil
	}
	return settings.EnabledPlugins, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
