package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
)

// DB persists installation records in ~/.nexo/installations.json.
// These records are what makes uninstall safe: nexo only deletes what
// it can prove it installed (plan D6), and the proof is here.
type DB struct {
	fs   ports.FS
	path string
}

type dbFile struct {
	SchemaVersion int                   `json:"schema_version"`
	Installations []domain.Installation `json:"installations"`
}

// OpenDB binds the database file under the state dir (~/.nexo).
func OpenDB(fsys ports.FS, stateDir string) *DB {
	return &DB{fs: fsys, path: path.Join(stateDir, "installations.json")}
}

// List returns all records; a missing file is an empty database.
func (d *DB) List() ([]domain.Installation, error) {
	data, err := d.fs.ReadFile(d.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file dbFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("corrupt installations db: %w", err)
	}
	return file.Installations, nil
}

// Find locates the record for one (asset, provider, target) key.
func (d *DB) Find(asset domain.ID, provider string, target domain.Target) (domain.Installation, bool, error) {
	records, err := d.List()
	if err != nil {
		return domain.Installation{}, false, err
	}
	for _, rec := range records {
		if rec.Asset == asset && rec.Provider == provider && rec.Target == target {
			return rec, true, nil
		}
	}
	return domain.Installation{}, false, nil
}

// Add inserts or replaces the record with the same key.
func (d *DB) Add(in domain.Installation) error {
	if err := in.Validate(); err != nil {
		return err
	}
	records, err := d.List()
	if err != nil {
		return err
	}
	out := records[:0:0]
	for _, rec := range records {
		if rec.Asset == in.Asset && rec.Provider == in.Provider && rec.Target == in.Target {
			continue
		}
		out = append(out, rec)
	}
	out = append(out, in)
	return d.save(out)
}

// Remove drops the record with the given key; absent keys are a no-op.
func (d *DB) Remove(asset domain.ID, provider string, target domain.Target) error {
	records, err := d.List()
	if err != nil {
		return err
	}
	out := records[:0:0]
	for _, rec := range records {
		if rec.Asset == asset && rec.Provider == provider && rec.Target == target {
			continue
		}
		out = append(out, rec)
	}
	return d.save(out)
}

func (d *DB) save(records []domain.Installation) error {
	data, err := json.MarshalIndent(dbFile{SchemaVersion: 1, Installations: records}, "", "  ")
	if err != nil {
		return err
	}
	if err := d.fs.MkdirAll(path.Dir(d.path), 0o755); err != nil {
		return err
	}
	return d.fs.WriteFile(d.path, data, 0o644)
}
