// Package tui is the interactive interface (spec §5). It holds ZERO
// business logic (spec §17): every screen reads through the same core
// services the CLI uses, and every action goes through the same
// installer. Complexity is revealed progressively (spec §33): the main
// menu shows five words, not the object model.
package tui

import (
	"fmt"
	"runtime"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

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

// Deps is everything the TUI needs, injected (plan D7).
type Deps struct {
	FS      ports.FS
	Paths   ports.Paths
	Version string
	Getwd   func() (string, error)
	Clock   ports.Clock
}

// Screen enumerates the TUI states.
type Screen int

const (
	ScreenMenu Screen = iota
	ScreenLibrary
	ScreenDetail
	ScreenProviders
	ScreenProject
	ScreenDoctor
	ScreenPickProvider
	ScreenConfirm
)

var menuItems = []string{"Library", "Providers", "This project", "Doctor", "Quit"}

// assetRow is one library asset plus its installation records.
type assetRow struct {
	asset    domain.Asset
	installs []domain.Installation
}

// pendingAction is an install/remove awaiting provider pick and/or
// confirmation.
type pendingAction struct {
	op       string // "install" | "remove"
	asset    domain.ID
	provider providers.Provider
}

// Model is the bubbletea model.
type Model struct {
	deps Deps

	Screen Screen
	Cursor int
	width  int
	height int

	lib       *library.Library
	db        *install.DB
	installer *install.Installer
	provs     []providers.Provider
	stateDir  string

	assets     []assetRow
	selected   assetRow
	provRows   []string
	projectRow []string
	project    string
	findings   []doctor.Finding

	candidates []providers.Provider
	pending    pendingAction

	status string
	err    error
}

// New builds the model and opens the core services.
func New(deps Deps) (Model, error) {
	store, err := config.Open(deps.FS, deps.Paths.StateDir())
	if err != nil {
		return Model{}, err
	}
	lib := library.New(deps.FS, store.Dir(), deps.Clock)
	db := install.OpenDB(deps.FS, store.Dir())
	installer := &install.Installer{
		FS:         deps.FS,
		Lib:        lib,
		DB:         db,
		Journal:    &tx.FileJournal{FS: deps.FS, Dir: store.JournalDir(), Clock: deps.Clock},
		Clock:      deps.Clock,
		NoSymlinks: runtime.GOOS == "windows",
	}
	return Model{
		deps:      deps,
		lib:       lib,
		db:        db,
		installer: installer,
		provs:     registry.All(deps.FS, deps.Paths),
		stateDir:  store.Dir(),
	}, nil
}

// Run launches the interactive program.
func Run(deps Deps) error {
	m, err := New(deps)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd { return nil }

// Update is the whole state machine. All service calls are local FS
// work, so they run inline — no async commands needed.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.Screen {
	case ScreenMenu:
		return m.updateMenu(key)
	case ScreenLibrary:
		return m.updateLibrary(key)
	case ScreenDetail:
		return m.updateDetail(key)
	case ScreenPickProvider:
		return m.updatePickProvider(key)
	case ScreenConfirm:
		return m.updateConfirm(key)
	default: // read-only screens: providers, project, doctor
		if key == "esc" || key == "q" {
			return m.toMenu(), nil
		}
		return m, nil
	}
}

func (m Model) toMenu() Model {
	m.Screen = ScreenMenu
	m.Cursor = 0
	m.err = nil
	return m
}

func (m Model) updateMenu(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(menuItems)-1 {
			m.Cursor++
		}
	case "enter":
		switch menuItems[m.Cursor] {
		case "Library":
			return m.enterLibrary(), nil
		case "Providers":
			return m.enterProviders(), nil
		case "This project":
			return m.enterProject(), nil
		case "Doctor":
			return m.enterDoctor(), nil
		case "Quit":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) enterLibrary() Model {
	m.Screen = ScreenLibrary
	m.Cursor = 0
	m.status = ""
	m.assets, m.err = m.loadAssets()
	return m
}

func (m Model) loadAssets() ([]assetRow, error) {
	assets, err := m.lib.List()
	if err != nil {
		return nil, err
	}
	records, err := m.db.List()
	if err != nil {
		return nil, err
	}
	byAsset := map[domain.ID][]domain.Installation{}
	for _, rec := range records {
		byAsset[rec.Asset] = append(byAsset[rec.Asset], rec)
	}
	rows := make([]assetRow, len(assets))
	for i, asset := range assets {
		rows[i] = assetRow{asset: asset, installs: byAsset[asset.ID]}
	}
	return rows, nil
}

func (m Model) updateLibrary(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		return m.toMenu(), nil
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(m.assets)-1 {
			m.Cursor++
		}
	case "enter":
		if len(m.assets) > 0 {
			m.selected = m.assets[m.Cursor]
			m.Screen = ScreenDetail
			m.status = ""
		}
	}
	return m, nil
}

func (m Model) updateDetail(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		return m.enterLibrary(), nil
	case "i": // install globally
		return m.startAction("install"), nil
	case "x": // remove global install
		return m.startAction("remove"), nil
	}
	return m, nil
}

// startAction begins an install/remove: pick a provider when more than
// one could apply (never guess — spec §12), then confirm.
func (m Model) startAction(op string) Model {
	m.pending = pendingAction{op: op, asset: m.selected.asset.ID}
	if op == "remove" {
		// Candidates: providers this asset is globally installed with.
		var cands []providers.Provider
		for _, rec := range m.selected.installs {
			if rec.Target.Scope != domain.ScopeGlobal {
				continue
			}
			for _, p := range m.provs {
				if p.ID() == rec.Provider {
					cands = append(cands, p)
				}
			}
		}
		if len(cands) == 0 {
			m.status = "not installed globally — nothing to remove"
			return m
		}
		m.candidates = cands
	} else {
		var cands []providers.Provider
		for _, p := range m.provs {
			if p.Capabilities().GlobalSkills && p.Detect().Installed {
				cands = append(cands, p)
			}
		}
		if len(cands) == 0 {
			m.status = "no detected provider supports global skills"
			return m
		}
		m.candidates = cands
	}
	if len(m.candidates) == 1 {
		m.pending.provider = m.candidates[0]
		m.Screen = ScreenConfirm
		return m
	}
	m.Screen = ScreenPickProvider
	m.Cursor = 0
	return m
}

func (m Model) updatePickProvider(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.Screen = ScreenDetail
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(m.candidates)-1 {
			m.Cursor++
		}
	case "enter":
		m.pending.provider = m.candidates[m.Cursor]
		m.Screen = ScreenConfirm
	}
	return m, nil
}

func (m Model) updateConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		m = m.runPending()
		m.Screen = ScreenDetail
	case "n", "esc", "q":
		m.status = "cancelled"
		m.Screen = ScreenDetail
	}
	return m, nil
}

// runPending executes the confirmed action through the same installer
// the CLI uses.
func (m Model) runPending() Model {
	req := install.Request{
		Asset:    m.pending.asset,
		Provider: m.pending.provider,
		Target:   domain.Target{Scope: domain.ScopeGlobal},
	}
	var err error
	var res install.Result
	if m.pending.op == "install" {
		res, err = m.installer.Install(req)
	} else {
		res, err = m.installer.Remove(req)
	}
	switch {
	case err != nil:
		m.status = "error: " + err.Error()
	case res.AlreadyInstalled:
		m.status = "already installed — nothing to do"
	case m.pending.op == "install":
		m.status = fmt.Sprintf("installed globally via %s", m.pending.provider.ID())
	default:
		m.status = fmt.Sprintf("removed global install (%s)", m.pending.provider.ID())
	}
	// Refresh the selected row's installations.
	if rows, lerr := m.loadAssets(); lerr == nil {
		m.assets = rows
		for _, row := range rows {
			if row.asset.ID == m.selected.asset.ID {
				m.selected = row
			}
		}
	}
	return m
}

func (m Model) enterProviders() Model {
	m.Screen = ScreenProviders
	m.provRows = nil
	for _, p := range m.provs {
		det := "not detected"
		if p.Detect().Installed {
			det = "detected"
		}
		caps := p.Capabilities()
		m.provRows = append(m.provRows, fmt.Sprintf("%-14s %-13s skills:%s plugins:%s",
			p.Name(), det, mark(caps.GlobalSkills || caps.ProjectSkills), mark(caps.Plugins)))
	}
	return m
}

func (m Model) enterProject() Model {
	m.Screen = ScreenProject
	m.projectRow = nil
	wd, err := m.deps.Getwd()
	if err != nil {
		m.err = err
		return m
	}
	m.project = wd
	for _, p := range m.provs {
		if !p.Detect().Installed {
			continue
		}
		assets, err := p.InspectProject(wd)
		if err != nil {
			m.projectRow = append(m.projectRow, fmt.Sprintf("%s: error: %v", p.ID(), err))
			continue
		}
		m.projectRow = append(m.projectRow, fmt.Sprintf("%s: %d asset(s)", p.ID(), len(assets)))
		sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
		for _, asset := range assets {
			m.projectRow = append(m.projectRow, fmt.Sprintf("  %-32s %s", asset.Name, asset.Type))
		}
	}
	return m
}

func (m Model) enterDoctor() Model {
	m.Screen = ScreenDoctor
	provMap := map[string]providers.Provider{}
	for _, p := range m.provs {
		provMap[p.ID()] = p
	}
	m.findings, m.err = doctor.Run(doctor.Deps{
		FS:       m.deps.FS,
		StateDir: m.stateDir,
		Lib:      m.lib,
		DB:       m.db,
		Provs:    provMap,
	})
	return m
}

func mark(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
