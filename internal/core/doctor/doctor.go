// Package doctor verifies that nexo's records still match reality and
// reports what drifted. It never repairs on its own — it names the
// problem and the command that fixes it, because silent repair of a
// user's config is how trust dies.
package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/core/install"
	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/core/treehash"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
)

// Finding is one detected problem.
type Finding struct {
	Severity string `json:"severity"` // "error" | "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Deps carries what doctor needs to inspect.
type Deps struct {
	FS       ports.FS
	StateDir string
	Lib      *library.Library
	DB       *install.DB
	Provs    map[string]providers.Provider // by ID
}

// Run performs all checks and returns findings (empty = healthy).
func Run(d Deps) ([]Finding, error) {
	var findings []Finding

	// 1. Interrupted or failed transactions left in the journal.
	journal := &tx.FileJournal{FS: d.FS, Dir: path.Join(d.StateDir, "journal")}
	pending, err := journal.Pending()
	if err != nil {
		return nil, fmt.Errorf("doctor: journal: %w", err)
	}
	for _, rec := range pending {
		findings = append(findings, Finding{
			Severity: "warning",
			Code:     "journal-" + rec.Status,
			Message:  fmt.Sprintf("transaction %q did not complete (%s) — inspect the paths in %s/journal and clean up", rec.Name, rec.Status, d.StateDir),
		})
	}

	// 2. Library must be readable (corrupt sidecars surface here).
	if _, err := d.Lib.List(); err != nil {
		findings = append(findings, Finding{
			Severity: "error",
			Code:     "library-unreadable",
			Message:  fmt.Sprintf("library: %v", err),
		})
	}

	// 3. Every installation record must still match reality (D2: the
	// hash decides, never the version).
	records, err := d.DB.List()
	if err != nil {
		return nil, fmt.Errorf("doctor: installations db: %w", err)
	}
	for _, rec := range records {
		findings = append(findings, checkRecord(d, rec)...)
	}
	return findings, nil
}

func checkRecord(d Deps, rec domain.Installation) []Finding {
	prov, ok := d.Provs[rec.Provider]
	if !ok {
		return []Finding{{
			Severity: "warning",
			Code:     "unknown-provider",
			Message:  fmt.Sprintf("%s: recorded provider %q is not registered", rec.Asset, rec.Provider),
		}}
	}
	locator, ok := prov.(providers.SkillLocator)
	if !ok {
		return nil
	}
	dest := path.Join(locator.SkillsDir(rec.Target), rec.Asset.Name)

	info, err := d.FS.Lstat(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return []Finding{{
			Severity: "error",
			Code:     "broken-missing",
			Message:  fmt.Sprintf("%s: recorded at %s but nothing is there — `nexo install %s` to restore or `nexo remove %s --force` to forget", rec.Asset, dest, rec.Asset, rec.Asset),
		}}
	}
	if err != nil {
		return []Finding{{Severity: "error", Code: "broken-unreadable", Message: fmt.Sprintf("%s: %v", rec.Asset, err)}}
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := d.FS.Readlink(dest)
		if err != nil || target != d.Lib.AssetPath(rec.Asset) {
			return []Finding{{
				Severity: "error",
				Code:     "broken-modified",
				Message:  fmt.Sprintf("%s: %s no longer points at the library", rec.Asset, dest),
			}}
		}
		if _, err := d.FS.Stat(dest); err != nil {
			return []Finding{{
				Severity: "error",
				Code:     "broken-dangling",
				Message:  fmt.Sprintf("%s: %s is a dangling link (asset removed from the library?)", rec.Asset, dest),
			}}
		}
		return nil
	}

	current, err := treehash.Tree(d.FS, dest)
	if err != nil {
		return []Finding{{Severity: "error", Code: "broken-unreadable", Message: fmt.Sprintf("%s: %v", rec.Asset, err)}}
	}
	if current != rec.Hash {
		return []Finding{{
			Severity: "error",
			Code:     "broken-modified",
			Message:  fmt.Sprintf("%s: content at %s changed since install (%s… vs %s…) — reinstall with --force or adopt the local edits", rec.Asset, dest, current[:8], rec.Hash[:8]),
		}}
	}
	return nil
}
