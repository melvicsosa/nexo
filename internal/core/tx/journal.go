package tx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/melvicsosa/nexo/internal/ports"
)

// Journal records transactions so an interrupted or failed run leaves
// evidence `nexo doctor` can surface instead of silent inconsistency.
type Journal interface {
	Begin(name string, steps []string) (id string, err error)
	Commit(id string) error
	Abort(id string, cause error) error
}

// Record is one journal entry as persisted on disk.
type Record struct {
	Name      string    `json:"name"`
	Steps     []string  `json:"steps"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"` // "pending" | "failed"
	Error     string    `json:"error,omitempty"`
}

// FileJournal persists records as JSON files in Dir. A committed
// transaction removes its record; anything left behind is either
// in-flight, crashed, or failed.
type FileJournal struct {
	FS    ports.FS
	Dir   string
	Clock ports.Clock
}

func (j *FileJournal) Begin(name string, steps []string) (string, error) {
	if err := j.FS.MkdirAll(j.Dir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%d-%s.json", j.Clock.Now().UnixNano(), sanitize(name))
	rec := Record{Name: name, Steps: steps, StartedAt: j.Clock.Now(), Status: "pending"}
	if err := j.write(id, rec); err != nil {
		return "", err
	}
	return id, nil
}

func (j *FileJournal) Commit(id string) error {
	return j.FS.Remove(path.Join(j.Dir, id))
}

func (j *FileJournal) Abort(id string, cause error) error {
	rec, err := j.read(id)
	if err != nil {
		return err
	}
	rec.Status = "failed"
	rec.Error = cause.Error()
	return j.write(id, rec)
}

// Pending lists leftover records: crashed or failed transactions.
func (j *FileJournal) Pending() ([]Record, error) {
	entries, err := j.FS.ReadDir(j.Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rec, err := j.read(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (j *FileJournal) write(id string, rec Record) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return j.FS.WriteFile(path.Join(j.Dir, id), data, 0o644)
}

func (j *FileJournal) read(id string) (Record, error) {
	data, err := j.FS.ReadFile(path.Join(j.Dir, id))
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, fmt.Errorf("journal %s: %w", id, err)
	}
	return rec, nil
}

func sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
}
