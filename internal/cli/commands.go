package cli

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/melvicsosa/nexo/internal/config"
	"github.com/melvicsosa/nexo/internal/core/doctor"
	"github.com/melvicsosa/nexo/internal/core/install"
	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/core/tx"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports"
	"github.com/melvicsosa/nexo/internal/providers"
	"github.com/melvicsosa/nexo/internal/providers/registry"
)

// parseArgs splits args into positionals and flags. boolFlags take no
// value; valueFlags consume the next argument (or --flag=value).
// Stdlib flag stops at the first positional, which fights the natural
// `nexo install <id> --global` order — this parser doesn't.
func parseArgs(args []string, boolFlags, valueFlags map[string]bool) ([]string, map[string]string, error) {
	var pos []string
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			pos = append(pos, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		switch {
		case boolFlags[name]:
			if hasValue {
				return nil, nil, fmt.Errorf("flag --%s takes no value", name)
			}
			flags[name] = "true"
		case valueFlags[name]:
			if !hasValue {
				i++
				if i >= len(args) {
					return nil, nil, fmt.Errorf("flag --%s needs a value", name)
				}
				value = args[i]
			}
			flags[name] = value
		default:
			return nil, nil, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return pos, flags, nil
}

// services opens the state store and builds the core services.
func (a *App) services() (*library.Library, *install.DB, *install.Installer, error) {
	store, err := config.Open(a.FS, a.Paths.StateDir())
	if err != nil {
		return nil, nil, nil, err
	}
	clock := a.clock()
	lib := library.New(a.FS, store.Dir(), clock)
	db := install.OpenDB(a.FS, store.Dir())
	installer := &install.Installer{
		FS:         a.FS,
		Lib:        lib,
		DB:         db,
		Journal:    &tx.FileJournal{FS: a.FS, Dir: store.JournalDir(), Clock: clock},
		Clock:      clock,
		NoSymlinks: runtime.GOOS == "windows",
	}
	return lib, db, installer, nil
}

func (a *App) clock() ports.Clock {
	if a.Clock != nil {
		return a.Clock
	}
	return ports.SystemClock{}
}

func (a *App) fail(err error) int {
	fmt.Fprintf(a.Stderr, "nexo: %v\n", err)
	return 1
}

// pickProvider resolves which provider an install/remove targets. An
// explicit --provider wins; otherwise exactly one detected+capable
// candidate is required — nexo never guesses between two tools that
// could both mean it (spec §12: ambiguity is the user's call).
func (a *App) pickProvider(idFlag string, scope domain.Scope) (providers.Provider, error) {
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
		caps := p.Capabilities()
		capable := (scope == domain.ScopeGlobal && caps.GlobalSkills) ||
			(scope == domain.ScopeProject && caps.ProjectSkills)
		if capable && p.Detect().Installed {
			candidates = append(candidates, p)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no detected provider supports this target")
	case 1:
		return candidates[0], nil
	default:
		ids := make([]string, len(candidates))
		for i, p := range candidates {
			ids[i] = p.ID()
		}
		return nil, fmt.Errorf("multiple providers could apply (%s) — pass --provider", strings.Join(ids, ", "))
	}
}

// resolveTarget turns --global/--project flags into a Target,
// defaulting to the current working directory as a project.
func (a *App) resolveTarget(flags map[string]string) (domain.Target, error) {
	if flags["global"] == "true" && flags["project"] != "" {
		return domain.Target{}, fmt.Errorf("--global and --project are mutually exclusive")
	}
	if flags["global"] == "true" {
		return domain.Target{Scope: domain.ScopeGlobal}, nil
	}
	projectPath := flags["project"]
	if projectPath == "" {
		wd, err := a.Getwd()
		if err != nil {
			return domain.Target{}, err
		}
		projectPath = wd
	}
	return domain.Target{Scope: domain.ScopeProject, ProjectPath: projectPath}, nil
}

// ---- nexo library ---------------------------------------------------------

func (a *App) cmdLibrary(args []string) int {
	_, flags, err := parseArgs(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return a.fail(err)
	}
	lib, db, _, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	assets, err := lib.List()
	if err != nil {
		return a.fail(err)
	}
	records, err := db.List()
	if err != nil {
		return a.fail(err)
	}
	installs := map[domain.ID]int{}
	for _, rec := range records {
		installs[rec.Asset]++
	}
	if flags["json"] == "true" {
		type row struct {
			domain.Asset
			Installations int `json:"installations"`
		}
		rows := make([]row, len(assets))
		for i, asset := range assets {
			rows[i] = row{Asset: asset, Installations: installs[asset.ID]}
		}
		return a.printJSON(rows)
	}
	if len(assets) == 0 {
		fmt.Fprintln(a.Stdout, "library is empty — `nexo adopt <name>` to add what you already have")
		return 0
	}
	w := tabwriter.NewWriter(a.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ASSET\tTYPE\tVERSION\tHASH\tINSTALLS")
	for _, asset := range assets {
		version := asset.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.8s\t%d\n", asset.ID, asset.Type, version, asset.Hash, installs[asset.ID])
	}
	w.Flush()
	return 0
}

// ---- nexo adopt -----------------------------------------------------------

func (a *App) cmdAdopt(args []string) int {
	pos, flags, err := parseArgs(args, map[string]bool{"json": true}, map[string]bool{"version": true, "description": true, "type": true})
	if err != nil {
		return a.fail(err)
	}
	if len(pos) != 1 {
		fmt.Fprintln(a.Stderr, "nexo: usage: nexo adopt <name-or-path> [--type skill|plugin] [--version v] [--description d]")
		return 2
	}
	lib, _, _, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	srcPath, name, err := a.resolveAdoptSource(pos[0])
	if err != nil {
		return a.fail(err)
	}
	assetType := flags["type"]
	if assetType == "" {
		assetType = string(a.detectAssetType(srcPath))
	}
	if !domain.Type(assetType).Valid() {
		return a.fail(fmt.Errorf("unknown asset type %q", assetType))
	}
	id := domain.ID{Source: domain.DefaultSource, Name: name}
	asset, err := lib.Add(srcPath, id, library.Sidecar{
		Type:        assetType,
		Version:     flags["version"],
		Description: flags["description"],
	})
	if err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		return a.printJSON(asset)
	}
	fmt.Fprintf(a.Stdout, "adopted %s (%.8s) from %s\n", asset.ID, asset.Hash, srcPath)
	return 0
}

// detectAssetType sniffs what kind of asset a directory holds: a
// Claude plugin manifest wins, otherwise it is treated as a skill.
func (a *App) detectAssetType(srcPath string) domain.Type {
	for _, marker := range []string{".claude-plugin/plugin.json", "plugin.json"} {
		if _, err := a.FS.Stat(srcPath + "/" + marker); err == nil {
			return domain.TypePlugin
		}
	}
	return domain.TypeSkill
}

// resolveAdoptSource accepts either a directory path or an asset name
// to search for across the current project and global inspections.
func (a *App) resolveAdoptSource(arg string) (srcPath, name string, err error) {
	if info, serr := a.FS.Stat(arg); serr == nil && info.IsDir() {
		parts := strings.Split(strings.TrimRight(arg, "/"), "/")
		return arg, parts[len(parts)-1], nil
	}
	wd, err := a.Getwd()
	if err != nil {
		return "", "", err
	}
	type candidate struct {
		path string
		hash string
	}
	var found []candidate
	seen := map[string]bool{}
	for _, p := range registry.All(a.FS, a.Paths) {
		if !p.Detect().Installed {
			continue
		}
		for _, scan := range [][]providers.FoundAsset{a.inspectQuiet(p, wd), a.inspectGlobalQuiet(p)} {
			for _, asset := range scan {
				if asset.Name != arg || asset.Type != domain.TypeSkill || seen[asset.Hash] {
					continue
				}
				seen[asset.Hash] = true
				found = append(found, candidate{path: asset.Path, hash: asset.Hash})
			}
		}
	}
	switch len(found) {
	case 0:
		return "", "", fmt.Errorf("no skill named %q found in this project or globally — pass a path instead", arg)
	case 1:
		return found[0].path, arg, nil
	default:
		paths := make([]string, len(found))
		for i, c := range found {
			paths[i] = c.path
		}
		return "", "", fmt.Errorf("%q exists with different content in multiple places — adopt one by path:\n  %s", arg, strings.Join(paths, "\n  "))
	}
}

func (a *App) inspectQuiet(p providers.Provider, projectPath string) []providers.FoundAsset {
	assets, _ := p.InspectProject(projectPath)
	return assets
}

func (a *App) inspectGlobalQuiet(p providers.Provider) []providers.FoundAsset {
	assets, _ := p.InspectGlobal()
	return assets
}

// ---- nexo install / remove ------------------------------------------------

var installBoolFlags = map[string]bool{"json": true, "global": true, "force": true, "dry-run": true}
var installValueFlags = map[string]bool{"provider": true, "project": true}

func (a *App) cmdInstall(args []string) int {
	return a.runInstallOp(args, "install")
}

func (a *App) cmdRemove(args []string) int {
	return a.runInstallOp(args, "remove")
}

func (a *App) runInstallOp(args []string, op string) int {
	pos, flags, err := parseArgs(args, installBoolFlags, installValueFlags)
	if err != nil {
		return a.fail(err)
	}
	if len(pos) != 1 {
		fmt.Fprintf(a.Stderr, "nexo: usage: nexo %s <asset> [--global|--project <path>] [--provider <id>] [--force] [--dry-run]\n", op)
		return 2
	}
	id, err := domain.ParseID(pos[0])
	if err != nil {
		return a.fail(err)
	}
	target, err := a.resolveTarget(flags)
	if err != nil {
		return a.fail(err)
	}
	prov, err := a.pickProvider(flags["provider"], target.Scope)
	if err != nil {
		return a.fail(err)
	}
	_, _, installer, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	req := install.Request{
		Asset:    id,
		Provider: prov,
		Target:   target,
		Force:    flags["force"] == "true",
		DryRun:   flags["dry-run"] == "true",
	}
	var result install.Result
	if op == "install" {
		result, err = installer.Install(req)
	} else {
		result, err = installer.Remove(req)
	}
	if err != nil {
		return a.fail(err)
	}
	if flags["json"] == "true" {
		return a.printJSON(result)
	}
	switch {
	case result.DryRun:
		fmt.Fprintf(a.Stdout, "dry run — %d planned steps:\n", len(result.Plan))
		for _, step := range result.Plan {
			fmt.Fprintf(a.Stdout, "  %s\n", step)
		}
	case result.AlreadyInstalled:
		fmt.Fprintf(a.Stdout, "%s is already installed — nothing to do\n", id)
	case op == "install":
		fmt.Fprintf(a.Stdout, "installed %s via %s (%s)\n", id, prov.ID(), describeTarget(target))
	default:
		fmt.Fprintf(a.Stdout, "removed %s from %s (%s)\n", id, prov.ID(), describeTarget(target))
	}
	return 0
}

func describeTarget(t domain.Target) string {
	if t.Scope == domain.ScopeGlobal {
		return "global"
	}
	return t.ProjectPath
}

// ---- nexo doctor ----------------------------------------------------------

func (a *App) cmdDoctor(args []string) int {
	_, flags, err := parseArgs(args, map[string]bool{"json": true}, nil)
	if err != nil {
		return a.fail(err)
	}
	lib, db, _, err := a.services()
	if err != nil {
		return a.fail(err)
	}
	provs := map[string]providers.Provider{}
	for _, p := range registry.All(a.FS, a.Paths) {
		provs[p.ID()] = p
	}
	findings, err := doctor.Run(doctor.Deps{
		FS:       a.FS,
		StateDir: a.Paths.StateDir(),
		Lib:      lib,
		DB:       db,
		Provs:    provs,
	})
	if err != nil {
		return a.fail(err)
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Severity < findings[j].Severity })
	if flags["json"] == "true" {
		if findings == nil {
			findings = []doctor.Finding{}
		}
		if code := a.printJSON(findings); code != 0 {
			return code
		}
	} else if len(findings) == 0 {
		fmt.Fprintln(a.Stdout, "everything checks out")
	} else {
		for _, f := range findings {
			fmt.Fprintf(a.Stdout, "%s [%s] %s\n", f.Severity, f.Code, f.Message)
		}
	}
	for _, f := range findings {
		if f.Severity == "error" {
			return 1
		}
	}
	return 0
}
