package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/melvicsosa/nexo/internal/core/library"
	"github.com/melvicsosa/nexo/internal/domain"
	"github.com/melvicsosa/nexo/internal/ports/portstest"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

// newModel builds a model over a machine with Claude Code only and one
// adopted library asset.
func newModel(t *testing.T) Model {
	t.Helper()
	m := portstest.NewMemFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.MkdirAll("/home/.claude", 0o755))
	must(m.MkdirAll("/repo", 0o755))
	must(m.MkdirAll("/src/wp", 0o755))
	must(m.WriteFile("/src/wp/SKILL.md", []byte("wp"), 0o644))

	deps := Deps{
		FS:      m,
		Paths:   portstest.FakePaths{HomeDir: "/home"},
		Version: "test",
		Getwd:   func() (string, error) { return "/repo", nil },
		Clock:   &portstest.FakeClock{T: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
	}
	model, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	lib := library.New(m, "/home/.nexo", deps.Clock)
	if _, err := lib.Add("/src/wp", domain.ID{Source: "local", Name: "wp"}, library.Sidecar{}); err != nil {
		t.Fatal(err)
	}
	return model
}

func TestMenuNavigation(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		wantScreen Screen
	}{
		{"enter library", []string{"enter"}, ScreenLibrary},
		{"down to providers", []string{"j", "enter"}, ScreenProviders},
		{"down to project", []string{"j", "j", "enter"}, ScreenProject},
		{"down to doctor", []string{"j", "j", "j", "enter"}, ScreenDoctor},
		{"cursor stops at top", []string{"k", "k", "enter"}, ScreenLibrary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, newModel(t), tt.keys...)
			if m.Screen != tt.wantScreen {
				t.Errorf("screen = %v, want %v", m.Screen, tt.wantScreen)
			}
		})
	}
}

func TestEscReturnsToMenu(t *testing.T) {
	for _, entry := range []string{"enter", "j enter", "j j enter", "j j j enter"} {
		m := press(t, newModel(t), strings.Fields(entry)...)
		m = press(t, m, "esc")
		if m.Screen != ScreenMenu {
			t.Errorf("after %q + esc: screen = %v, want menu", entry, m.Screen)
		}
	}
}

func TestLibraryToDetail(t *testing.T) {
	m := press(t, newModel(t), "enter") // library
	if len(m.assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(m.assets))
	}
	m = press(t, m, "enter") // detail
	if m.Screen != ScreenDetail || m.selected.asset.ID.Name != "wp" {
		t.Fatalf("detail: screen=%v selected=%v", m.Screen, m.selected.asset.ID)
	}
	view := m.View()
	for _, want := range []string{"local/wp", "unversioned", "Installations"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view missing %q:\n%s", want, view)
		}
	}
}

func TestInstallFlowSingleProviderConfirm(t *testing.T) {
	m := press(t, newModel(t), "enter", "enter") // library → detail
	// One detected provider (claude-code) → straight to confirm.
	m = press(t, m, "i")
	if m.Screen != ScreenConfirm {
		t.Fatalf("screen = %v, want confirm", m.Screen)
	}
	m = press(t, m, "y")
	if m.Screen != ScreenDetail || !strings.Contains(m.status, "installed globally via claude-code") {
		t.Fatalf("after confirm: screen=%v status=%q", m.Screen, m.status)
	}
	// The row now shows Global.
	if got := statusOf(m.selected); got != "Global" {
		t.Errorf("status = %q, want Global", got)
	}
}

func TestInstallCancelDoesNothing(t *testing.T) {
	m := press(t, newModel(t), "enter", "enter", "i", "n")
	if m.Screen != ScreenDetail || m.status != "cancelled" {
		t.Fatalf("screen=%v status=%q", m.Screen, m.status)
	}
	if len(m.selected.installs) != 0 {
		t.Error("cancelled action still installed")
	}
}

func TestRemoveFlow(t *testing.T) {
	m := press(t, newModel(t), "enter", "enter", "i", "y") // install first
	m = press(t, m, "x", "y")                              // then remove
	if !strings.Contains(m.status, "removed global install") {
		t.Fatalf("status = %q", m.status)
	}
	if got := statusOf(m.selected); got != "Library" {
		t.Errorf("status = %q, want Library", got)
	}
}

func TestRemoveWithoutInstallExplains(t *testing.T) {
	m := press(t, newModel(t), "enter", "enter", "x")
	if m.Screen != ScreenDetail || !strings.Contains(m.status, "nothing to remove") {
		t.Fatalf("screen=%v status=%q", m.Screen, m.status)
	}
}

func TestDoctorScreenHealthy(t *testing.T) {
	m := press(t, newModel(t), "j", "j", "j", "enter")
	if m.Screen != ScreenDoctor || len(m.findings) != 0 {
		t.Fatalf("screen=%v findings=%v", m.Screen, m.findings)
	}
	if !strings.Contains(m.View(), "everything checks out") {
		t.Errorf("doctor view:\n%s", m.View())
	}
}
