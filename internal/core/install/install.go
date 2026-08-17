// Package install is the write engine of nexo: it turns "put this
// library asset there" into a transactional plan and executes it.
//
// The safety rules it enforces:
//
//	D3 — global installs symlink into the library (one source of
//	     truth); project installs copy (the repo stays self-contained).
//	D5 — never touch .gitignore; refuse to overwrite different content
//	     without --force; identical content is a no-op.
//	D6 — only delete what nexo installed, verified by hash. Detected
//	     assets need adopt or an explicit --force.
package install

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/core/treehash"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
)

// copyIgnore keeps nexo metadata and noise out of installed copies.
var copyIgnore = map[string]bool{
	treehash.SidecarName: true,
	".DS_Store":          true,
	".git":               true,
}

// Installer wires the pieces an install/remove needs.
type Installer struct {
	FS      ports.FS
	Lib     *library.Library
	DB      *DB
	Journal tx.Journal
	Clock   ports.Clock
	// NoSymlinks switches global installs from symlink to copy —
	// Windows, where creating symlinks needs elevated privileges.
	// Wired from runtime.GOOS at the edges, never decided here.
	NoSymlinks bool
}

// Request describes one install or remove operation.
type Request struct {
	Asset    domain.ID
	Provider providers.Provider
	Target   domain.Target
	Force    bool
	DryRun   bool
}

// Result reports what happened (or, for a dry run, what would).
type Result struct {
	Plan             []string `json:"plan"`
	AlreadyInstalled bool     `json:"already_installed,omitempty"`
	Removed          bool     `json:"removed,omitempty"`
	DryRun           bool     `json:"dry_run,omitempty"`
}

// Install puts a library asset into the target through the provider.
func (i *Installer) Install(req Request) (Result, error) {
	locator, err := i.locator(req)
	if err != nil {
		return Result{}, err
	}
	asset, err := i.Lib.Get(req.Asset)
	if err != nil {
		return Result{}, err
	}
	libPath := i.Lib.AssetPath(req.Asset)
	destDir := locator.SkillsDir(req.Target)
	dest := path.Join(destDir, req.Asset.Name)

	var steps []tx.Step
	strategy := domain.StrategyMaterialize

	if req.Target.Scope == domain.ScopeGlobal && !i.NoSymlinks {
		// D3: global = symlink into the library.
		done, preSteps, err := i.planGlobalSymlink(dest, libPath, req.Force)
		if err != nil {
			return Result{}, err
		}
		if done {
			return i.recordOnly(req, asset, strategy)
		}
		steps = append(steps, tx.MkdirAll(destDir, 0o755))
		steps = append(steps, preSteps...)
		steps = append(steps, tx.Symlink(libPath, dest))
	} else if req.Target.Scope == domain.ScopeGlobal {
		// Windows fallback: global installs are copies too.
		done, preSteps, err := i.planProjectCopy(dest, asset.Hash, req.Force)
		if err != nil {
			return Result{}, err
		}
		if done {
			return i.recordOnly(req, asset, strategy)
		}
		steps = append(steps, preSteps...)
		copySteps, err := tx.PlanCopyTree(i.FS, libPath, dest, copyIgnore)
		if err != nil {
			return Result{}, err
		}
		steps = append(steps, copySteps...)
	} else {
		// D3: project = copy, self-contained and committable.
		done, preSteps, err := i.planProjectCopy(dest, asset.Hash, req.Force)
		if err != nil {
			return Result{}, err
		}
		if done {
			return i.recordOnly(req, asset, strategy)
		}
		steps = append(steps, preSteps...)
		copySteps, err := tx.PlanCopyTree(i.FS, libPath, dest, copyIgnore)
		if err != nil {
			return Result{}, err
		}
		steps = append(steps, copySteps...)
		mSteps, err := manifestSteps(i.FS, req.Target.ProjectPath, func(m *Manifest) {
			upsertEntry(m, ManifestEntry{
				ID: asset.ID, Type: asset.Type, Provider: req.Provider.ID(),
				Hash: asset.Hash, Version: asset.Version,
			})
		})
		if err != nil {
			return Result{}, err
		}
		steps = append(steps, mSteps...)
	}

	if req.DryRun {
		return Result{Plan: tx.DryRun(steps), DryRun: true}, nil
	}
	engine := &tx.Engine{FS: i.FS, Journal: i.Journal}
	if err := engine.Run("install "+req.Asset.String(), steps); err != nil {
		return Result{}, err
	}
	if err := i.record(req, asset, strategy); err != nil {
		return Result{}, fmt.Errorf("installed, but recording failed (run nexo doctor): %w", err)
	}
	return Result{Plan: tx.DryRun(steps)}, nil
}

// planGlobalSymlink decides what to do about an existing destination
// for a symlink install. done=true means already correctly installed.
func (i *Installer) planGlobalSymlink(dest, libPath string, force bool) (done bool, pre []tx.Step, err error) {
	info, lerr := i.FS.Lstat(dest)
	if errors.Is(lerr, fs.ErrNotExist) {
		return false, nil, nil
	}
	if lerr != nil {
		return false, nil, lerr
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		if target, rerr := i.FS.Readlink(dest); rerr == nil && target == libPath {
			return true, nil, nil // already the right link
		}
	}
	if !force {
		return false, nil, fmt.Errorf("%s already exists and was not installed by nexo — remove it, adopt it, or pass --force", dest)
	}
	pre, err = tx.PlanRemoveTree(i.FS, dest)
	return false, pre, err
}

// planProjectCopy decides what to do about an existing destination for
// a copy install (D5). done=true means identical content already there.
func (i *Installer) planProjectCopy(dest, wantHash string, force bool) (done bool, pre []tx.Step, err error) {
	existing, herr := treehash.Tree(i.FS, dest)
	if errors.Is(herr, fs.ErrNotExist) {
		return false, nil, nil
	}
	if herr != nil {
		return false, nil, herr
	}
	if existing == wantHash {
		return true, nil, nil // identical content: no-op, just record
	}
	if !force {
		return false, nil, fmt.Errorf("%s exists with different content (%s… vs %s…) — pass --force to overwrite", dest, existing[:8], wantHash[:8])
	}
	pre, err = tx.PlanRemoveTree(i.FS, dest)
	return false, pre, err
}

// Remove uninstalls an asset from a target (D6: hash-verified).
func (i *Installer) Remove(req Request) (Result, error) {
	locator, err := i.locator(req)
	if err != nil {
		return Result{}, err
	}
	dest := path.Join(locator.SkillsDir(req.Target), req.Asset.Name)
	rec, managed, err := i.DB.Find(req.Asset, req.Provider.ID(), req.Target)
	if err != nil {
		return Result{}, err
	}
	if !managed && !req.Force {
		return Result{}, fmt.Errorf("%s at %s was not installed by nexo — `nexo adopt` it first, remove it manually, or pass --force", req.Asset, dest)
	}

	var steps []tx.Step
	_, lerr := i.FS.Lstat(dest)
	present := lerr == nil
	if present {
		if managed && !req.Force {
			if err := i.verifyManaged(rec, dest); err != nil {
				return Result{}, err
			}
		}
		steps, err = tx.PlanRemoveTree(i.FS, dest)
		if err != nil {
			return Result{}, err
		}
	} else if !managed {
		return Result{}, fmt.Errorf("%s: nothing at %s", req.Asset, dest)
	}

	if req.Target.Scope == domain.ScopeProject {
		mSteps, err := manifestSteps(i.FS, req.Target.ProjectPath, func(m *Manifest) {
			dropEntry(m, req.Asset, req.Provider.ID())
		})
		if err != nil {
			return Result{}, err
		}
		steps = append(steps, mSteps...)
	}

	if req.DryRun {
		return Result{Plan: tx.DryRun(steps), DryRun: true, Removed: present}, nil
	}
	engine := &tx.Engine{FS: i.FS, Journal: i.Journal}
	if err := engine.Run("remove "+req.Asset.String(), steps); err != nil {
		return Result{}, err
	}
	if err := i.DB.Remove(req.Asset, req.Provider.ID(), req.Target); err != nil {
		return Result{}, fmt.Errorf("removed, but the record survived (run nexo doctor): %w", err)
	}
	return Result{Plan: tx.DryRun(steps), Removed: present}, nil
}

// verifyManaged proves the destination still matches what nexo
// installed before deleting it (D6).
func (i *Installer) verifyManaged(rec domain.Installation, dest string) error {
	info, err := i.FS.Lstat(dest)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := i.FS.Readlink(dest)
		if err != nil {
			return err
		}
		if target != i.Lib.AssetPath(rec.Asset) {
			return fmt.Errorf("%s points at %s, not at the library — modified since install; pass --force to remove anyway", dest, target)
		}
		return nil
	}
	current, err := treehash.Tree(i.FS, dest)
	if err != nil {
		return err
	}
	if current != rec.Hash {
		return fmt.Errorf("%s was modified since nexo installed it (%s… vs %s…) — pass --force to remove anyway", dest, current[:8], rec.Hash[:8])
	}
	return nil
}

// locator validates capability and target, and returns the provider's
// write-side location contract.
func (i *Installer) locator(req Request) (providers.SkillLocator, error) {
	if err := req.Target.Validate(); err != nil {
		return nil, err
	}
	caps := req.Provider.Capabilities()
	if req.Target.Scope == domain.ScopeGlobal && !caps.GlobalSkills {
		return nil, fmt.Errorf("provider %s does not support global skills", req.Provider.ID())
	}
	if req.Target.Scope == domain.ScopeProject && !caps.ProjectSkills {
		return nil, fmt.Errorf("provider %s does not support project skills", req.Provider.ID())
	}
	locator, ok := req.Provider.(providers.SkillLocator)
	if !ok {
		return nil, fmt.Errorf("provider %s cannot locate skill installs", req.Provider.ID())
	}
	return locator, nil
}

func (i *Installer) recordOnly(req Request, asset domain.Asset, strategy domain.Strategy) (Result, error) {
	if req.DryRun {
		return Result{AlreadyInstalled: true, DryRun: true}, nil
	}
	if err := i.record(req, asset, strategy); err != nil {
		return Result{}, err
	}
	return Result{AlreadyInstalled: true}, nil
}

func (i *Installer) record(req Request, asset domain.Asset, strategy domain.Strategy) error {
	return i.DB.Add(domain.Installation{
		Asset:       asset.ID,
		Hash:        asset.Hash,
		Version:     asset.Version,
		Provider:    req.Provider.ID(),
		Target:      req.Target,
		Strategy:    strategy,
		Source:      domain.SourceLibrary,
		InstalledAt: i.Clock.Now(),
	})
}
