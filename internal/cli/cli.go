// Package cli implements nexo's non-interactive commands. It holds no
// business logic (spec §17): it wires ports into core services and
// providers, renders results, and exits. Every listing command supports
// --json so scripts and CI consume the same data humans see.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
	"github.com/melvicsosa/nexo/internal/providers/registry"
)

// App carries the injected dependencies for a CLI run.
type App struct {
	FS      ports.FS
	Paths   ports.Paths
	Stdout  io.Writer
	Stderr  io.Writer
	Version string
	Getwd   func() (string, error)
	Clock   ports.Clock // nil = system clock
	// LaunchTUI starts the interactive UI; nil (tests, non-TTY) falls
	// back to usage. Wired in main so the cli package stays free of
	// terminal concerns.
	LaunchTUI func() error
}

// Run dispatches args and returns the process exit code.
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		if a.LaunchTUI != nil {
			if err := a.LaunchTUI(); err != nil {
				return a.fail(err)
			}
			return 0
		}
		a.usage()
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "-version", "--version", "-v":
		fmt.Fprintf(a.Stdout, "nexo %s\n", a.Version)
		return 0
	case "help", "-h", "--help":
		a.usage()
		return 0
	case "providers":
		return a.cmdProviders(rest)
	case "list":
		return a.cmdList(rest)
	case "project":
		return a.cmdProject(rest)
	case "library":
		return a.cmdLibrary(rest)
	case "adopt":
		return a.cmdAdopt(rest)
	case "install":
		return a.cmdInstall(rest)
	case "remove":
		return a.cmdRemove(rest)
	case "doctor":
		return a.cmdDoctor(rest)
	case "plugin":
		return a.cmdPlugin(rest)
	case "marketplace":
		return a.cmdMarketplace(rest)
	case "ui":
		if a.LaunchTUI == nil {
			fmt.Fprintln(a.Stderr, "nexo: interactive UI needs a terminal")
			return 1
		}
		if err := a.LaunchTUI(); err != nil {
			return a.fail(err)
		}
		return 0
	default:
		fmt.Fprintf(a.Stderr, "nexo: unknown command %q\n\n", cmd)
		a.usage()
		return 2
	}
}

func (a *App) usage() {
	fmt.Fprint(a.Stdout, `nexo — package manager for AI development assets

Usage:
  nexo providers [--json]              detected providers and their capabilities
  nexo list [--json]                   global assets per provider
  nexo project inspect [path] [--json] assets configured in a project
  nexo library [--json]                assets in your local library
  nexo adopt <name-or-path>            bring an existing asset into the library
  nexo install <asset> [flags]         install a library asset
  nexo remove <asset> [flags]          uninstall a nexo-managed asset
  nexo doctor [--json]                 verify records against reality
  nexo plugin enable|disable <name>    flip a plugin in the provider's config
  nexo marketplace sync                expose library plugins to Claude Code
  nexo version                         print version
  nexo help                            this help

Install/remove flags:
  --global | --project <path>   target (default: current directory as project)
  --provider <id>               required when more than one provider applies
  --force                       override safety checks (overwrite/modified)
  --dry-run                     print the plan without touching anything
`)
}

// hasJSON strips a --json flag from args.
func hasJSON(args []string) (bool, []string) {
	rest := args[:0:0]
	found := false
	for _, arg := range args {
		if arg == "--json" {
			found = true
			continue
		}
		rest = append(rest, arg)
	}
	return found, rest
}

func (a *App) printJSON(v any) int {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(a.Stderr, "nexo: %v\n", err)
		return 1
	}
	return 0
}

// ---- providers ------------------------------------------------------------

type providerReport struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Detection    providers.DetectionResult `json:"detection"`
	Capabilities providers.Capabilities    `json:"capabilities"`
}

func (a *App) cmdProviders(args []string) int {
	asJSON, _ := hasJSON(args)
	var reports []providerReport
	for _, p := range registry.All(a.FS, a.Paths) {
		reports = append(reports, providerReport{
			ID:           p.ID(),
			Name:         p.Name(),
			Detection:    p.Detect(),
			Capabilities: p.Capabilities(),
		})
	}
	if asJSON {
		return a.printJSON(reports)
	}
	w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tDETECTED\tSKILLS\tPLUGINS")
	for _, r := range reports {
		detected := "no"
		if r.Detection.Installed {
			detected = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, detected,
			yesNo(r.Capabilities.GlobalSkills || r.Capabilities.ProjectSkills),
			yesNo(r.Capabilities.Plugins))
	}
	w.Flush()
	return 0
}

// ---- list -----------------------------------------------------------------

type globalReport struct {
	Provider string                 `json:"provider"`
	Detected bool                   `json:"detected"`
	Assets   []providers.FoundAsset `json:"assets"`
	Error    string                 `json:"error,omitempty"`
}

func (a *App) cmdList(args []string) int {
	asJSON, _ := hasJSON(args)
	var reports []globalReport
	for _, p := range registry.All(a.FS, a.Paths) {
		rep := globalReport{Provider: p.ID(), Detected: p.Detect().Installed}
		if rep.Detected {
			assets, err := p.InspectGlobal()
			if err != nil {
				rep.Error = err.Error()
			}
			rep.Assets = assets
		}
		reports = append(reports, rep)
	}
	if asJSON {
		return a.printJSON(reports)
	}
	exit := 0
	for _, rep := range reports {
		if !rep.Detected {
			fmt.Fprintf(a.Stdout, "%s: not detected\n", rep.Provider)
			continue
		}
		fmt.Fprintf(a.Stdout, "%s (%d assets)\n", rep.Provider, len(rep.Assets))
		w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
		for _, asset := range rep.Assets {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", asset.Name, asset.Type, describe(asset))
		}
		w.Flush()
		if rep.Error != "" {
			fmt.Fprintf(a.Stderr, "nexo: %s: %s\n", rep.Provider, rep.Error)
			exit = 1
		}
	}
	return exit
}

// ---- project inspect ------------------------------------------------------

type inspectReport struct {
	Project   string         `json:"project"`
	Providers []globalReport `json:"providers"`
}

func (a *App) cmdProject(args []string) int {
	if len(args) == 0 || args[0] != "inspect" {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo project inspect [path] [--json]")
		return 2
	}
	asJSON, rest := hasJSON(args[1:])
	projectPath := ""
	if len(rest) > 0 {
		projectPath = rest[0]
	} else {
		wd, err := a.Getwd()
		if err != nil {
			fmt.Fprintf(a.Stderr, "nexo: %v\n", err)
			return 1
		}
		projectPath = wd
	}

	report := inspectReport{Project: projectPath}
	for _, p := range registry.All(a.FS, a.Paths) {
		rep := globalReport{Provider: p.ID(), Detected: p.Detect().Installed}
		if rep.Detected {
			assets, err := p.InspectProject(projectPath)
			if err != nil {
				rep.Error = err.Error()
			}
			rep.Assets = assets
		}
		report.Providers = append(report.Providers, rep)
	}
	if asJSON {
		return a.printJSON(report)
	}
	fmt.Fprintf(a.Stdout, "Project: %s\n\n", report.Project)
	exit := 0
	for _, rep := range report.Providers {
		if !rep.Detected {
			fmt.Fprintf(a.Stdout, "%s: not detected\n", rep.Provider)
			continue
		}
		fmt.Fprintf(a.Stdout, "%s (%d assets)\n", rep.Provider, len(rep.Assets))
		w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
		for _, asset := range rep.Assets {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", asset.Name, asset.Type, describe(asset))
		}
		w.Flush()
		if rep.Error != "" {
			fmt.Fprintf(a.Stderr, "nexo: %s: %s\n", rep.Provider, rep.Error)
			exit = 1
		}
	}
	return exit
}

// ---- rendering helpers ----------------------------------------------------

func describe(asset providers.FoundAsset) string {
	switch {
	case asset.Enabled != nil && *asset.Enabled:
		return withVersion("enabled", asset.Version)
	case asset.Enabled != nil:
		return withVersion("disabled", asset.Version)
	case asset.Version != "":
		return asset.Version
	default:
		return ""
	}
}

func withVersion(state, version string) string {
	if version == "" {
		return state
	}
	return state + " " + version
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
