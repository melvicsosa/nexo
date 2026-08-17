// Package registries implements asset registries (spec §14): sources
// you download FROM, distinct from the Library you install FROM.
//
//	Registry → fetch → Library → install → Project/Global
//
// A registry is a directory (a git clone works — clone it yourself and
// add the path; nexo stays exec-free) containing an index:
//
//	<registry>/index.json
//	  {"schema_version": 1, "assets": [
//	    {"name": "wordpress-review", "type": "skill", "version": "1.2.0",
//	     "description": "...", "path": "./assets/wordpress-review"}]}
//
// Fetched assets land in the library namespaced by registry name
// (plan D10): company/wordpress-review never collides with
// local/wordpress-review.
package registries

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
)

// Entry is one configured registry.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Index is a registry's asset catalog.
type Index struct {
	SchemaVersion int          `json:"schema_version"`
	Assets        []IndexEntry `json:"assets"`
}

// IndexEntry describes one asset offered by a registry.
type IndexEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
}

// Hit is a search result.
type Hit struct {
	Registry string     `json:"registry"`
	Entry    IndexEntry `json:"entry"`
}

type configFile struct {
	SchemaVersion int     `json:"schema_version"`
	Registries    []Entry `json:"registries"`
}

// Store persists the configured registries in ~/.nexo/registries.json.
type Store struct {
	fs   ports.FS
	path string
}

// Open binds the store under the state dir.
func Open(fsys ports.FS, stateDir string) *Store {
	return &Store{fs: fsys, path: path.Join(stateDir, "registries.json")}
}

// List returns the configured registries; missing file = none.
func (s *Store) List() ([]Entry, error) {
	data, err := s.fs.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file configFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("corrupt registries config: %w", err)
	}
	return file.Registries, nil
}

// Add validates and registers a registry. The name becomes the asset
// namespace, so it must be a valid ID source segment; the index must
// load NOW — a registry that cannot be read should fail at add time,
// not at first search.
func (s *Store) Add(name, dir string) error {
	if err := (domain.ID{Source: name, Name: "x"}).Validate(); err != nil {
		return fmt.Errorf("invalid registry name %q: must be usable as an asset namespace", name)
	}
	if name == domain.DefaultSource {
		return fmt.Errorf("%q is reserved for locally adopted assets", domain.DefaultSource)
	}
	if _, err := LoadIndex(s.fs, dir); err != nil {
		return fmt.Errorf("registry %q: %w", name, err)
	}
	entries, err := s.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == name {
			return fmt.Errorf("registry %q already configured (%s)", name, e.Path)
		}
	}
	return s.save(append(entries, Entry{Name: name, Path: dir}))
}

// Remove forgets a registry. Fetched assets stay in the library — the
// registry is a source, not an owner.
func (s *Store) Remove(name string) error {
	entries, err := s.List()
	if err != nil {
		return err
	}
	out := entries[:0:0]
	found := false
	for _, e := range entries {
		if e.Name == name {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return fmt.Errorf("registry %q is not configured", name)
	}
	return s.save(out)
}

// Search matches term against asset names and descriptions across all
// configured registries. An unreadable registry surfaces as an error —
// silently skipping it would make "no results" a lie.
func (s *Store) Search(term string) ([]Hit, error) {
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	term = strings.ToLower(term)
	var hits []Hit
	for _, reg := range entries {
		index, err := LoadIndex(s.fs, reg.Path)
		if err != nil {
			return nil, fmt.Errorf("registry %q: %w", reg.Name, err)
		}
		for _, asset := range index.Assets {
			if strings.Contains(strings.ToLower(asset.Name), term) ||
				strings.Contains(strings.ToLower(asset.Description), term) {
				hits = append(hits, Hit{Registry: reg.Name, Entry: asset})
			}
		}
	}
	return hits, nil
}

// Resolve finds one asset in one registry and returns the absolute
// path of its content.
func (s *Store) Resolve(registry, name string) (IndexEntry, string, error) {
	entries, err := s.List()
	if err != nil {
		return IndexEntry{}, "", err
	}
	for _, reg := range entries {
		if reg.Name != registry {
			continue
		}
		index, err := LoadIndex(s.fs, reg.Path)
		if err != nil {
			return IndexEntry{}, "", fmt.Errorf("registry %q: %w", registry, err)
		}
		for _, asset := range index.Assets {
			if asset.Name == name {
				return asset, path.Join(reg.Path, asset.Path), nil
			}
		}
		return IndexEntry{}, "", fmt.Errorf("registry %q has no asset %q", registry, name)
	}
	return IndexEntry{}, "", fmt.Errorf("registry %q is not configured — `nexo registry add %s <path>`", registry, registry)
}

// LoadIndex reads and validates a registry's index.json.
func LoadIndex(fsys ports.FS, dir string) (Index, error) {
	data, err := fsys.ReadFile(path.Join(dir, "index.json"))
	if err != nil {
		return Index{}, fmt.Errorf("cannot read index.json in %s: %w", dir, err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("invalid index.json in %s: %w", dir, err)
	}
	for _, asset := range index.Assets {
		if asset.Name == "" || asset.Path == "" {
			return Index{}, fmt.Errorf("index.json in %s: every asset needs name and path", dir)
		}
		if strings.HasPrefix(asset.Path, "/") || strings.Contains(asset.Path, "..") {
			return Index{}, fmt.Errorf("index.json in %s: asset %q path must stay inside the registry", dir, asset.Name)
		}
	}
	return index, nil
}

func (s *Store) save(entries []Entry) error {
	data, err := json.MarshalIndent(configFile{SchemaVersion: 1, Registries: entries}, "", "  ")
	if err != nil {
		return err
	}
	if err := s.fs.MkdirAll(path.Dir(s.path), 0o755); err != nil {
		return err
	}
	return s.fs.WriteFile(s.path, data, 0o644)
}
