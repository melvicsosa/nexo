package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/melvicsosa/nexo/internal/domain"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Bold(true)
	okStyle     = lipgloss.NewStyle()
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("nexo %s", m.deps.Version)))
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errStyle.Render("error: "+m.err.Error()) + "\n\n")
	}
	switch m.Screen {
	case ScreenMenu:
		m.viewMenu(&b)
	case ScreenLibrary:
		m.viewLibrary(&b)
	case ScreenDetail:
		m.viewDetail(&b)
	case ScreenProviders:
		m.viewList(&b, "Providers", m.provRows)
	case ScreenProject:
		m.viewList(&b, "Project: "+m.project, m.projectRow)
	case ScreenDoctor:
		m.viewDoctor(&b)
	case ScreenPickProvider:
		m.viewPickProvider(&b)
	case ScreenConfirm:
		m.viewConfirm(&b)
	}
	return b.String()
}

func (m Model) viewMenu(b *strings.Builder) {
	for i, item := range menuItems {
		prefix := "  "
		line := item
		if i == m.Cursor {
			prefix = "> "
			line = cursorStyle.Render(item)
		}
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
	b.WriteString("\n" + dimStyle.Render("↑/↓ move · enter select · q quit"))
}

func (m Model) viewLibrary(b *strings.Builder) {
	b.WriteString("Library\n\n")
	if len(m.assets) == 0 {
		b.WriteString(dimStyle.Render("empty — `nexo adopt <name>` to add what you already have") + "\n")
	}
	for i, row := range m.assets {
		prefix := "  "
		line := fmt.Sprintf("%-32s %-7s %s", row.asset.ID, row.asset.Type, statusOf(row))
		if i == m.Cursor {
			prefix = "> "
			line = cursorStyle.Render(line)
		}
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
	if m.status != "" {
		b.WriteString("\n" + okStyle.Render(m.status) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("enter detail · esc back"))
}

// statusOf renders the most useful single fact (spec §6): Global,
// Project, Multiple or Library.
func statusOf(row assetRow) string {
	global, project := 0, 0
	for _, rec := range row.installs {
		if rec.Target.Scope == domain.ScopeGlobal {
			global++
		} else {
			project++
		}
	}
	switch {
	case global > 0 && project > 0:
		return "Multiple"
	case global > 0:
		return "Global"
	case project > 0:
		return "Project"
	default:
		return "Library"
	}
}

func (m Model) viewDetail(b *strings.Builder) {
	asset := m.selected.asset
	fmt.Fprintf(b, "%s\n\n", cursorStyle.Render(asset.ID.String()))
	version := asset.Version
	if version == "" {
		version = "unversioned"
	}
	fmt.Fprintf(b, "Type:        %s\n", asset.Type)
	fmt.Fprintf(b, "Version:     %s\n", version)
	fmt.Fprintf(b, "Hash:        %.12s\n", asset.Hash)
	if asset.Description != "" {
		fmt.Fprintf(b, "Description: %s\n", asset.Description)
	}
	b.WriteString("\nInstallations\n")
	if len(m.selected.installs) == 0 {
		b.WriteString(dimStyle.Render("  none") + "\n")
	}
	for _, rec := range m.selected.installs {
		where := "global"
		if rec.Target.Scope == domain.ScopeProject {
			where = rec.Target.ProjectPath
		}
		fmt.Fprintf(b, "  %-12s %s\n", rec.Provider, where)
	}
	if m.status != "" {
		b.WriteString("\n" + okStyle.Render(m.status) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("i install globally · x remove global · esc back"))
	b.WriteString("\n" + dimStyle.Render("project installs: nexo install "+asset.ID.Name+" --project <path>"))
}

func (m Model) viewList(b *strings.Builder, title string, rows []string) {
	b.WriteString(title + "\n\n")
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("nothing to show") + "\n")
	}
	for _, row := range rows {
		b.WriteString("  " + row + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("esc back"))
}

func (m Model) viewDoctor(b *strings.Builder) {
	b.WriteString("Doctor\n\n")
	if len(m.findings) == 0 && m.err == nil {
		b.WriteString(okStyle.Render("everything checks out") + "\n")
	}
	for _, f := range m.findings {
		fmt.Fprintf(b, "  %s [%s] %s\n", f.Severity, f.Code, f.Message)
	}
	b.WriteString("\n" + dimStyle.Render("esc back"))
}

func (m Model) viewPickProvider(b *strings.Builder) {
	fmt.Fprintf(b, "%s %s with which provider?\n\n", m.pending.op, m.pending.asset)
	for i, p := range m.candidates {
		prefix := "  "
		line := p.Name()
		if i == m.Cursor {
			prefix = "> "
			line = cursorStyle.Render(line)
		}
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
	b.WriteString("\n" + dimStyle.Render("enter select · esc cancel"))
}

func (m Model) viewConfirm(b *strings.Builder) {
	verb := "Install"
	if m.pending.op == "remove" {
		verb = "Remove"
	}
	fmt.Fprintf(b, "%s %s globally via %s?\n\n", verb, m.pending.asset, m.pending.provider.Name())
	b.WriteString(dimStyle.Render("y confirm · n cancel"))
}
