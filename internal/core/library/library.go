// Package library is the user's local collection of assets (spec §2.2):
// what you HAVE, independent of where it is installed. Deleting an
// installation never touches the Library; deleting from the Library
// never touches installations. Layout:
//
//	~/.nexo/library/<source>/<name>/     asset content
//	~/.nexo/library/<source>/<name>/.nexo.json   sidecar metadata
//
// The sidecar carries optional metadata (version, description); the
// asset's identity is always the content hash, computed live so
// library-side edits are reflected immediately (plan D2).
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/melvicsosa/nexo/internal/core/treehash"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
)

// Sidecar is the persisted per-asset metadata.
type Sidecar struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Version     string    `json:"version,omitempty"`
	Description string    `json:"description,omitempty"`
	AddedAt     time.Time `json:"added_at"`
}

// Library manages ~/.nexo/library.
type Library struct {
	fs    ports.FS
	root  string
	clock ports.Clock
}

// New builds a Library rooted under the given state dir (~/.nexo).
func New(fsys ports.FS, stateDir string, clock ports.Clock) *Library {
	return &Library{fs: fsys, root: path.Join(stateDir, "library"), clock: clock}
}

// AssetPath is where an asset's content lives in the library.
func (l *Library) AssetPath(id domain.ID) string {
	return path.Join(l.root, id.Source, id.Name)
}

// Add copies the tree at srcPath into the library under id, writing the
// sidecar. The copy goes through the tx engine so a failed add leaves
// no partial asset behind. Adding over an existing asset with different
// content is refused — update is an explicit Remove+Add in v1.
func (l *Library) Add(srcPath string, id domain.ID, meta Sidecar) (domain.Asset, error) {
	if err := id.Validate(); err != nil {
		return domain.Asset{}, err
	}
	srcHash, err := treehash.Tree(l.fs, srcPath)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("library add %s: %w", id, err)
	}
	dst := l.AssetPath(id)
	if existing, err := treehash.Tree(l.fs, dst); err == nil {
		if existing == srcHash {
			return l.Get(id) // identical content already in the library
		}
		return domain.Asset{}, fmt.Errorf("library add %s: already exists with different content (library %s…, source %s…) — remove it first", id, short(existing), short(srcHash))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return domain.Asset{}, fmt.Errorf("library add %s: %w", id, err)
	}

	steps, err := tx.PlanCopyTree(l.fs, srcPath, dst, map[string]bool{
		treehash.SidecarName: true, // never import someone else's sidecar
		".DS_Store":          true,
		".git":               true,
	})
	if err != nil {
		return domain.Asset{}, fmt.Errorf("library add %s: %w", id, err)
	}
	meta.ID = id.String()
	if meta.Type == "" {
		meta.Type = string(domain.TypeSkill)
	}
	meta.AddedAt = l.clock.Now()
	sidecarData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return domain.Asset{}, err
	}
	steps = append(steps, tx.WriteFile(path.Join(dst, treehash.SidecarName), sidecarData, 0o644))

	engine := &tx.Engine{FS: l.fs}
	if err := engine.Run("library-add "+id.String(), steps); err != nil {
		return domain.Asset{}, err
	}
	return l.Get(id)
}

// Get loads one asset: sidecar plus live content hash.
func (l *Library) Get(id domain.ID) (domain.Asset, error) {
	dir := l.AssetPath(id)
	hash, err := treehash.Tree(l.fs, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.Asset{}, fmt.Errorf("library: %s is not in the library", id)
		}
		return domain.Asset{}, err
	}
	meta, err := l.readSidecar(dir)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("library: %s: %w", id, err)
	}
	return domain.Asset{
		ID:          id,
		Type:        domain.Type(meta.Type),
		Version:     meta.Version,
		Description: meta.Description,
		Hash:        hash,
	}, nil
}

// List returns every asset in the library, sorted by ID.
func (l *Library) List() ([]domain.Asset, error) {
	sources, err := l.fs.ReadDir(l.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []domain.Asset
	for _, srcEntry := range sources {
		if !srcEntry.IsDir() {
			continue
		}
		names, err := l.fs.ReadDir(path.Join(l.root, srcEntry.Name()))
		if err != nil {
			return nil, err
		}
		for _, nameEntry := range names {
			if !nameEntry.IsDir() {
				continue
			}
			id := domain.ID{Source: srcEntry.Name(), Name: nameEntry.Name()}
			asset, err := l.Get(id)
			if err != nil {
				return nil, err
			}
			out = append(out, asset)
		}
	}
	return out, nil
}

// Remove deletes an asset from the library. Installations are not
// touched: the Library and installations are independent by design
// (spec §2.2) — global symlinks into the removed asset will show up as
// Broken in doctor, which is the truthful outcome.
func (l *Library) Remove(id domain.ID) error {
	dir := l.AssetPath(id)
	if _, err := l.fs.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("library: %s is not in the library", id)
	}
	steps, err := tx.PlanRemoveTree(l.fs, dir)
	if err != nil {
		return err
	}
	engine := &tx.Engine{FS: l.fs}
	return engine.Run("library-remove "+id.String(), steps)
}

func (l *Library) readSidecar(dir string) (Sidecar, error) {
	data, err := l.fs.ReadFile(path.Join(dir, treehash.SidecarName))
	if errors.Is(err, fs.ErrNotExist) {
		// Asset content without metadata is legal (hand-copied into
		// the library); identity is the hash anyway.
		return Sidecar{Type: string(domain.TypeSkill)}, nil
	}
	if err != nil {
		return Sidecar{}, err
	}
	var meta Sidecar
	if err := json.Unmarshal(data, &meta); err != nil {
		return Sidecar{}, fmt.Errorf("corrupt sidecar: %w", err)
	}
	return meta, nil
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
