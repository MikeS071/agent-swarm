package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelUpdateNavigationAndModes(t *testing.T) {
	m := model{
		viewMode: "list",
		tickets:  []ticketRow{{ID: "sw-01"}, {ID: "sw-02"}, {ID: "sw-03"}},
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.viewMode != "detail" {
		t.Fatalf("expected detail mode, got %q", m.viewMode)
	}
	if m.detailID != "sw-02" {
		t.Fatalf("expected detailID sw-02, got %q", m.detailID)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.viewMode != "list" {
		t.Fatalf("expected list mode after esc, got %q", m.viewMode)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if !m.compact {
		t.Fatal("expected compact mode to toggle on")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if _, ok := next.(model); !ok {
		t.Fatal("expected model return")
	}
}

func TestModelUpdateCursorBounds(t *testing.T) {
	m := model{tickets: []ticketRow{{ID: "sw-01"}, {ID: "sw-02"}}}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if m.cursor != 0 {
		t.Fatalf("expected cursor to stay at 0, got %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor to stay at max index 1, got %d", m.cursor)
	}
}
