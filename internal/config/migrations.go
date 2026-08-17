package config

import (
	"fmt"

	"github.com/melvicsosa/nexo/internal/ports"
)

// migration upgrades the store layout from exactly `from` to from+1.
type migration struct {
	from  int
	apply func(fsys ports.FS, dir string) error
}

// migrations is the ordered upgrade chain. Empty today — it exists from
// day one so the first real layout change is a one-entry diff, not an
// architectural event.
var migrations = []migration{}

func migrate(fsys ports.FS, dir string, st State) (State, error) {
	for st.SchemaVersion < SchemaVersion {
		var found *migration
		for i := range migrations {
			if migrations[i].from == st.SchemaVersion {
				found = &migrations[i]
				break
			}
		}
		if found == nil {
			return st, fmt.Errorf("no migration path from schema %d", st.SchemaVersion)
		}
		if err := found.apply(fsys, dir); err != nil {
			return st, err
		}
		st.SchemaVersion++
	}
	return st, nil
}
