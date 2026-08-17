package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
)

// PlanPluginEnable plans flipping one plugin's enable state in the
// right settings.json (Reference strategy, plan D1). The file is
// parsed into a generic map so every field nexo does not understand is
// preserved verbatim — we edit exactly one key, atomically, through
// the tx engine. Project scope edits <project>/.claude/settings.json
// (the team-shared file, matching where enabledPlugins lives globally).
func (a *Adapter) PlanPluginEnable(plugin string, target domain.Target, enabled bool) ([]tx.Step, bool, error) {
	if err := target.Validate(); err != nil {
		return nil, false, err
	}
	settingsPath := path.Join(a.root(), "settings.json")
	if target.Scope == domain.ScopeProject {
		settingsPath = path.Join(target.ProjectPath, ".claude", "settings.json")
	}

	settings := map[string]any{}
	data, err := a.fs.ReadFile(settingsPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A missing settings file is legal; we create a minimal one.
	case err != nil:
		return nil, false, fmt.Errorf("claude-code: %s: %w", settingsPath, err)
	default:
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, false, fmt.Errorf("claude-code: %s is not valid JSON — refusing to touch it: %w", settingsPath, err)
		}
	}

	enabledPlugins := map[string]any{}
	if raw, ok := settings["enabledPlugins"]; ok {
		cast, ok := raw.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("claude-code: %s: enabledPlugins is not an object — refusing to touch it", settingsPath)
		}
		enabledPlugins = cast
	}
	if current, ok := enabledPlugins[plugin]; ok {
		if state, ok := current.(bool); ok && state == enabled {
			return nil, false, nil // already what was asked
		}
	} else if !enabled {
		return nil, false, nil // absent means disabled already
	}
	enabledPlugins[plugin] = enabled
	settings["enabledPlugins"] = enabledPlugins

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, false, err
	}
	out = append(out, '\n')
	steps := []tx.Step{
		tx.MkdirAll(path.Dir(settingsPath), 0o755),
		tx.WriteFile(settingsPath, out, 0o644),
	}
	return steps, true, nil
}
