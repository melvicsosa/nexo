// Package config owns nexo's own metadata directory (~/.nexo). It
// carries a schema_version from the very first write (plan Phase 1) so
// future layout changes are migrations, never guesswork.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/ports"
)

// SchemaVersion is the layout version this binary writes and expects.
const SchemaVersion = 1

const stateFile = "state.json"

// State is the persisted root metadata of the store.
type State struct {
	SchemaVersion int `json:"schema_version"`
}

// Store is nexo's metadata directory, opened and migrated.
type Store struct {
	fs    ports.FS
	dir   string
	state State
}

// Open initializes dir on first use (writing SchemaVersion) and runs
// pending migrations on subsequent opens. It refuses state written by
// a NEWER nexo: downgrading over data we do not understand is how
// metadata gets corrupted.
func Open(fsys ports.FS, dir string) (*Store, error) {
	if err := fsys.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	p := path.Join(dir, stateFile)

	data, err := fsys.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		st := State{SchemaVersion: SchemaVersion}
		if err := writeState(fsys, p, st); err != nil {
			return nil, err
		}
		return &Store{fs: fsys, dir: dir, state: st}, nil
	case err != nil:
		return nil, fmt.Errorf("config: %w", err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("config: corrupt %s: %w", stateFile, err)
	}
	if st.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("config: state has schema %d but this nexo understands up to %d — written by a newer nexo, upgrade first", st.SchemaVersion, SchemaVersion)
	}
	if st.SchemaVersion < SchemaVersion {
		migrated, err := migrate(fsys, dir, st)
		if err != nil {
			return nil, fmt.Errorf("config: migrating schema %d -> %d: %w", st.SchemaVersion, SchemaVersion, err)
		}
		if err := writeState(fsys, p, migrated); err != nil {
			return nil, err
		}
		st = migrated
	}
	return &Store{fs: fsys, dir: dir, state: st}, nil
}

// Dir is the store's root directory.
func (s *Store) Dir() string { return s.dir }

// JournalDir is where the transaction journal lives.
func (s *Store) JournalDir() string { return path.Join(s.dir, "journal") }

// State returns the loaded (post-migration) state.
func (s *Store) State() State { return s.state }

func writeState(fsys ports.FS, p string, st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := fsys.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}
