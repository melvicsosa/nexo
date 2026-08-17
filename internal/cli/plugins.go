package cli

import (
	"fmt"

	"github.com/melvicsosa/nexo/internal/config"
	"github.com/melvicsosa/nexo/internal/core/market"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/providers"
	"github.com/melvicsosa/nexo/internal/providers/registry"
)

// cmdPlugin handles `nexo plugin enable|disable <name>` — the
// Reference strategy in action: nexo flips exactly one key in the
// provider's own configuration, transactionally, and never touches the
// provider's plugin cache or install records.
func (a *App) cmdPlugin(args []string) int {
	if len(args) == 0 || (args[0] != "enable" && args[0] != "disable") {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo plugin enable|disable <name> [--global|--project <path>] [--provider <id>] [--dry-run]")
		return 2
	}
	op := args[0]
	pos, flags, err := parseArgs(args[1:], installBoolFlags, installValueFlags)
	if err != nil {
		return a.fail(err)
	}
	if len(pos) != 1 {
		fmt.Fprintf(a.Stderr, "nexo: usage: nexo plugin %s <name> [flags]\n", op)
		return 2
	}
	plugin := pos[0]
	target, err := a.resolveTarget(flags)
	if err != nil {
		return a.fail(err)
	}
	prov, err := a.pickPluginProvider(flags["provider"])
	if err != nil {
		return a.fail(err)
	}
	configurator, ok := prov.(providers.PluginConfigurator)
	if !ok {
		return a.fail(fmt.Errorf("provider %s cannot configure plugins", prov.ID()))
	}
	steps, changed, err := configurator.PlanPluginEnable(plugin, target, op == "enable")
	if err != nil {
		return a.fail(err)
	}

	type outcome struct {
		Plugin  string   `json:"plugin"`
		Enabled bool     `json:"enabled"`
		Changed bool     `json:"changed"`
		Plan    []string `json:"plan,omitempty"`
		DryRun  bool     `json:"dry_run,omitempty"`
	}
	result := outcome{Plugin: plugin, Enabled: op == "enable", Changed: changed, Plan: tx.DryRun(steps)}

	if !changed {
		if flags["json"] == "true" {
			return a.printJSON(result)
		}
		fmt.Fprintf(a.Stdout, "%s is already %sd — nothing to do\n", plugin, op)
		return 0
	}
	if flags["dry-run"] == "true" {
		result.DryRun = true
		if flags["json"] == "true" {
			return a.printJSON(result)
		}
		fmt.Fprintf(a.Stdout, "dry run — %d planned steps:\n", len(result.Plan))
		for _, step := range result.Plan {
			fmt.Fprintf(a.Stdout, "  %s\n", step)
		}
		return 0
	}

	store, err := config.Open(a.FS, a.Paths.StateDir())
	if err != nil {
		return a.fail(err)
	}
	engine := &tx.Engine{FS: a.FS, Journal: &tx.FileJournal{FS: a.FS, Dir: store.JournalDir(), Clock: a.clock()}}
	if err := engine.Run("plugin-"+op+" "+plugin, steps); err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.Stdout, "%sd %s via %s (%s)\n", op, plugin, prov.ID(), describeTarget(target))
	return 0
}

// pickPluginProvider mirrors pickProvider but for the plugin
// capability.
func (a *App) pickPluginProvider(idFlag string) (providers.Provider, error) {
	all := registry.All(a.FS, a.Paths)
	if idFlag != "" {
		for _, p := range all {
			if p.ID() == idFlag {
				return p, nil
			}
		}
		return nil, fmt.Errorf("unknown provider %q", idFlag)
	}
	var candidates []providers.Provider
	for _, p := range all {
		if p.Capabilities().Plugins && p.Detect().Installed {
			candidates = append(candidates, p)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no detected provider supports plugins")
	case 1:
		return candidates[0], nil
	default:
		return nil, fmt.Errorf("multiple providers support plugins — pass --provider")
	}
}

// cmdMarketplace handles `nexo marketplace sync`: regenerate the local
// Claude Code marketplace from the library's plugin assets (plan D1).
func (a *App) cmdMarketplace(args []string) int {
	if len(args) == 0 || args[0] != "sync" {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo marketplace sync [--json]")
		return 2
	}
	_, flags, err := parseArgs(args[1:], map[string]bool{"json": true, "dry-run": true}, nil)
	if err != nil {
		return a.fail(err)
	}
	lib, _, _, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	store, err := config.Open(a.FS, a.Paths.StateDir())
	if err != nil {
		return a.fail(err)
	}
	steps, names, err := market.SyncSteps(a.FS, lib, store.Dir(), "nexo")
	if err != nil {
		return a.fail(err)
	}

	type outcome struct {
		Marketplace string   `json:"marketplace"`
		Dir         string   `json:"dir"`
		Plugins     []string `json:"plugins"`
		Plan        []string `json:"plan,omitempty"`
		DryRun      bool     `json:"dry_run,omitempty"`
	}
	result := outcome{Marketplace: market.Name, Dir: market.Dir(store.Dir()), Plugins: names, Plan: tx.DryRun(steps)}

	if flags["dry-run"] == "true" {
		result.DryRun = true
		if flags["json"] == "true" {
			return a.printJSON(result)
		}
		for _, step := range result.Plan {
			fmt.Fprintf(a.Stdout, "  %s\n", step)
		}
		return 0
	}
	engine := &tx.Engine{FS: a.FS, Journal: &tx.FileJournal{FS: a.FS, Dir: store.JournalDir(), Clock: a.clock()}}
	if err := engine.Run("marketplace-sync", steps); err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.Stdout, "marketplace %q synced: %d plugin(s) at %s\n", market.Name, len(names), result.Dir)
	if len(names) > 0 {
		fmt.Fprintf(a.Stdout, "\nRegister it once in Claude Code:\n  claude plugin marketplace add %s\nThen install natively:\n  claude plugin install <name>@%s\n", result.Dir, market.Name)
	} else {
		fmt.Fprintln(a.Stdout, "no plugin assets in the library yet — `nexo adopt <path> --type plugin` to add one")
	}
	return 0
}

var _ = domain.TypePlugin // keep import stable while commands evolve
