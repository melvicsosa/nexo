package domain

import (
	"fmt"
	"time"
)

// Scope says where an installation lives.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// Target is the destination of an installation: the global location of
// a provider, or a specific project directory.
type Target struct {
	Scope       Scope
	ProjectPath string // required for ScopeProject, must be empty for ScopeGlobal
}

// Validate enforces the scope/path pairing.
func (t Target) Validate() error {
	switch t.Scope {
	case ScopeGlobal:
		if t.ProjectPath != "" {
			return fmt.Errorf("global target must not carry a project path (got %q)", t.ProjectPath)
		}
	case ScopeProject:
		if t.ProjectPath == "" {
			return fmt.Errorf("project target requires a project path")
		}
	default:
		return fmt.Errorf("unknown scope %q", t.Scope)
	}
	return nil
}

// Strategy is how an installation is realized (plan D1). Materialize
// writes files (skills); Reference mutates provider configuration
// (Claude Code plugins) — the provider's own mechanism stays in charge.
type Strategy string

const (
	StrategyMaterialize Strategy = "materialize"
	StrategyReference   Strategy = "reference"
)

// Valid reports whether s is a known strategy.
func (s Strategy) Valid() bool {
	return s == StrategyMaterialize || s == StrategyReference
}

// HealthState classifies what inspection finds at a location (spec §11).
type HealthState string

const (
	// HealthManaged: installed by nexo and hash-verified intact.
	HealthManaged HealthState = "managed"
	// HealthDetected: exists but was not installed by nexo. Never
	// deleted by nexo (plan D6) — offer adopt or require --force.
	HealthDetected HealthState = "detected"
	// HealthUnknown: the provider reports something nexo cannot parse.
	HealthUnknown HealthState = "unknown"
	// HealthBroken: nexo's records say installed, but the files or
	// config no longer match (decided by hash, plan D2).
	HealthBroken HealthState = "broken"
)

// InstallSource records where an installation came from.
type InstallSource string

const (
	SourceLibrary  InstallSource = "library"
	SourceRegistry InstallSource = "registry"
)

// Installation is the record of an asset applied to a target through a
// provider. Hash pins the exact content that was installed: uninstall
// and drift detection compare against it, never against the version.
type Installation struct {
	Asset       ID
	Hash        string
	Version     string // optional, informational only
	Provider    string
	Target      Target
	Strategy    Strategy
	Source      InstallSource
	InstalledAt time.Time
}

// Validate checks the record is complete enough to be acted on safely.
func (in Installation) Validate() error {
	if err := in.Asset.Validate(); err != nil {
		return err
	}
	if in.Hash == "" {
		return fmt.Errorf("installation of %s: missing content hash", in.Asset)
	}
	if in.Provider == "" {
		return fmt.Errorf("installation of %s: missing provider", in.Asset)
	}
	if err := in.Target.Validate(); err != nil {
		return fmt.Errorf("installation of %s: %w", in.Asset, err)
	}
	if !in.Strategy.Valid() {
		return fmt.Errorf("installation of %s: unknown strategy %q", in.Asset, in.Strategy)
	}
	return nil
}
