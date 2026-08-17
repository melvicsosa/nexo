package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/melvicsosa/nexo/internal/config"
	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/core/registries"
	"github.com/melvicsosa/nexo/internal/domain"
)

func (a *App) registriesStore() (*registries.Store, error) {
	store, err := config.Open(a.FS, a.Paths.StateDir())
	if err != nil {
		return nil, err
	}
	return registries.Open(a.FS, store.Dir()), nil
}

// cmdRegistry handles `nexo registry add|list|remove`.
func (a *App) cmdRegistry(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo registry add <name> <path> | list [--json] | remove <name>")
		return 2
	}
	store, err := a.registriesStore()
	if err != nil {
		return a.fail(err)
	}
	switch args[0] {
	case "add":
		pos, _, err := parseArgs(args[1:], nil, nil)
		if err != nil || len(pos) != 2 {
			fmt.Fprintln(a.Stderr, "nexo: usage: nexo registry add <name> <path>")
			return 2
		}
		if err := store.Add(pos[0], pos[1]); err != nil {
			return a.fail(err)
		}
		fmt.Fprintf(a.Stdout, "registry %q added — `nexo search <term>` to explore it\n", pos[0])
		return 0
	case "list":
		_, flags, err := parseArgs(args[1:], map[string]bool{"json": true}, nil)
		if err != nil {
			return a.fail(err)
		}
		entries, err := store.List()
		if err != nil {
			return a.fail(err)
		}
		if flags["json"] == "true" {
			if entries == nil {
				entries = []registries.Entry{}
			}
			return a.printJSON(entries)
		}
		if len(entries) == 0 {
			fmt.Fprintln(a.Stdout, "no registries configured — `nexo registry add <name> <path>`")
			return 0
		}
		w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPATH")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\n", e.Name, e.Path)
		}
		w.Flush()
		return 0
	case "remove":
		pos, _, err := parseArgs(args[1:], nil, nil)
		if err != nil || len(pos) != 1 {
			fmt.Fprintln(a.Stderr, "nexo: usage: nexo registry remove <name>")
			return 2
		}
		if err := store.Remove(pos[0]); err != nil {
			return a.fail(err)
		}
		fmt.Fprintf(a.Stdout, "registry %q removed (fetched assets stay in the library)\n", pos[0])
		return 0
	default:
		fmt.Fprintf(a.Stderr, "nexo: unknown registry subcommand %q\n", args[0])
		return 2
	}
}

// cmdSearch handles `nexo search <term>` across configured registries.
func (a *App) cmdSearch(args []string) int {
	pos, flags, err := parseArgs(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return a.fail(err)
	}
	if len(pos) != 1 {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo search <term> [--json]")
		return 2
	}
	store, err := a.registriesStore()
	if err != nil {
		return a.fail(err)
	}
	hits, err := store.Search(pos[0])
	if err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		if hits == nil {
			hits = []registries.Hit{}
		}
		return a.printJSON(hits)
	}
	if len(hits) == 0 {
		fmt.Fprintf(a.Stdout, "no assets matching %q\n", pos[0])
		return 0
	}
	w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ASSET\tTYPE\tVERSION\tDESCRIPTION")
	for _, h := range hits {
		version := h.Entry.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(w, "%s/%s\t%s\t%s\t%s\n", h.Registry, h.Entry.Name, h.Entry.Type, version, h.Entry.Description)
	}
	w.Flush()
	return 0
}

// cmdFetch handles `nexo fetch <registry>/<name>`: registry → library
// (spec §14). Installing is a separate, explicit step.
func (a *App) cmdFetch(args []string) int {
	pos, flags, err := parseArgs(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return a.fail(err)
	}
	if len(pos) != 1 || !strings.Contains(pos[0], "/") {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo fetch <registry>/<name>")
		return 2
	}
	id, err := domain.ParseID(pos[0])
	if err != nil {
		return a.fail(err)
	}
	store, err := a.registriesStore()
	if err != nil {
		return a.fail(err)
	}
	entry, assetDir, err := store.Resolve(id.Source, id.Name)
	if err != nil {
		return a.fail(err)
	}
	lib, _, _, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	assetType := entry.Type
	if assetType == "" {
		assetType = string(domain.TypeSkill)
	}
	asset, err := lib.Add(assetDir, id, library.Sidecar{
		Type:        assetType,
		Version:     entry.Version,
		Description: entry.Description,
	})
	if err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		return a.printJSON(asset)
	}
	fmt.Fprintf(a.Stdout, "fetched %s (%.8s) into the library — `nexo install %s` to use it\n", asset.ID, asset.Hash, asset.ID)
	return 0
}
