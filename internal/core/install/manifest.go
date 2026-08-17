package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
)

// Manifest is the committable record of what nexo installed into a
// project (spec §19): reproducibility, team sharing, drift detection.
// The provider's native configuration remains the runtime source of
// truth — this file records intent.
type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	Assets        []ManifestEntry `json:"assets"`
}

// ManifestEntry pins one installed asset.
type ManifestEntry struct {
	ID       domain.ID   `json:"id"`
	Type     domain.Type `json:"type"`
	Provider string      `json:"provider"`
	Hash     string      `json:"hash"`
	Version  string      `json:"version,omitempty"`
}

func manifestPath(projectPath string) string {
	return path.Join(projectPath, ".nexo", "manifest.json")
}

// LoadManifest reads a project manifest; missing means empty.
func LoadManifest(fsys ports.FS, projectPath string) (Manifest, error) {
	data, err := fsys.ReadFile(manifestPath(projectPath))
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{SchemaVersion: 1}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("corrupt project manifest: %w", err)
	}
	return m, nil
}

// manifestSteps plans the manifest update as part of the install
// transaction, so a failed install also rolls the manifest back.
func manifestSteps(fsys ports.FS, projectPath string, mutate func(*Manifest)) ([]tx.Step, error) {
	m, err := LoadManifest(fsys, projectPath)
	if err != nil {
		return nil, err
	}
	mutate(&m)
	sort.Slice(m.Assets, func(i, j int) bool {
		if m.Assets[i].ID.String() != m.Assets[j].ID.String() {
			return m.Assets[i].ID.String() < m.Assets[j].ID.String()
		}
		return m.Assets[i].Provider < m.Assets[j].Provider
	})
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return []tx.Step{
		tx.MkdirAll(path.Join(projectPath, ".nexo"), 0o755),
		tx.WriteFile(manifestPath(projectPath), data, 0o644),
	}, nil
}

func upsertEntry(m *Manifest, entry ManifestEntry) {
	for i := range m.Assets {
		if m.Assets[i].ID == entry.ID && m.Assets[i].Provider == entry.Provider {
			m.Assets[i] = entry
			return
		}
	}
	m.Assets = append(m.Assets, entry)
}

func dropEntry(m *Manifest, id domain.ID, provider string) {
	out := m.Assets[:0]
	for _, e := range m.Assets {
		if e.ID == id && e.Provider == provider {
			continue
		}
		out = append(out, e)
	}
	m.Assets = out
}
