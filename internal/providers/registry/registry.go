// Package registry is the single place where provider adapters are
// wired in. Adding a provider to nexo is one new package under
// internal/providers/ plus one entry in All — that is the entire
// extension surface (plan §2).
package registry

import (
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
	"github.com/melvicsosa/nexo/internal/providers/claudecode"
	"github.com/melvicsosa/nexo/internal/providers/cursor"
)

// All returns every registered provider adapter with its ports
// injected.
func All(fsys ports.FS, paths ports.Paths) []providers.Provider {
	return []providers.Provider{
		claudecode.New(fsys, paths),
		cursor.New(fsys, paths),
		// codex: seat reserved — Phase 6, after its research spike.
	}
}
