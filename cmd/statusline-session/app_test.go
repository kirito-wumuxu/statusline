package main

import (
	"context"
	"testing"

	"statusline/internal/session"
	"statusline/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
)

type mockSessionService struct {
	items []session.SessionItem
}

func (m *mockSessionService) ListSessions(ctx context.Context, onlyLocalNode bool) ([]session.SessionItem, error) {
	if onlyLocalNode {
		var local []session.SessionItem
		for _, it := range m.items {
			if it.IsLocalNode {
				local = append(local, it)
			}
		}
		return local, nil
	}
	return m.items, nil
}

func (m *mockSessionService) GetLocalNodeID() string {
	return "node_test_local"
}

func TestAppModel_Navigation(t *testing.T) {
	items := []session.SessionItem{
		{
			Record:      storage.SessionRecord{SessionID: "sess_1", ProductType: "antigravity"},
			DisplayName: "repo-1",
			IsLocalNode: true,
		},
		{
			Record:      storage.SessionRecord{SessionID: "sess_2", ProductType: "claude_code"},
			DisplayName: "repo-2",
			IsLocalNode: true,
		},
	}

	mockSvc := &mockSessionService{items: items}
	app := NewAppModel(mockSvc, true)
	app.termWidth = 100
	app.termHeight = 30
	app.loadSessions()

	if len(app.filteredItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(app.filteredItems))
	}
	if app.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", app.cursor)
	}

	// Test Down key
	downMsg := tea.KeyMsg{Type: tea.KeyDown}
	m, _ := app.Update(downMsg)
	app = m.(*AppModel)
	if app.cursor != 1 {
		t.Fatalf("expected cursor 1 after Down, got %d", app.cursor)
	}

	// Test Up key
	upMsg := tea.KeyMsg{Type: tea.KeyUp}
	m, _ = app.Update(upMsg)
	app = m.(*AppModel)
	if app.cursor != 0 {
		t.Fatalf("expected cursor 0 after Up, got %d", app.cursor)
	}
}

func TestAppModel_ToggleNodeScope(t *testing.T) {
	items := []session.SessionItem{
		{
			Record:      storage.SessionRecord{SessionID: "sess_local", ProductType: "antigravity"},
			DisplayName: "repo-local",
			IsLocalNode: true,
		},
		{
			Record:      storage.SessionRecord{SessionID: "sess_foreign", ProductType: "claude_code"},
			DisplayName: "repo-foreign",
			IsLocalNode: false,
		},
	}

	mockSvc := &mockSessionService{items: items}
	app := NewAppModel(mockSvc, true) // start with onlyLocal = true
	app.loadSessions()

	if len(app.filteredItems) != 1 {
		t.Fatalf("expected 1 local item, got %d", len(app.filteredItems))
	}

	// Press 'a' to toggle to all nodes
	aMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	m, _ := app.Update(aMsg)
	app = m.(*AppModel)

	if app.onlyLocalNode {
		t.Errorf("expected onlyLocalNode=false after toggle")
	}
	if len(app.filteredItems) != 2 {
		t.Fatalf("expected 2 items after toggling to all nodes, got %d", len(app.filteredItems))
	}
}

func TestAppModel_EnterLaunches(t *testing.T) {
	items := []session.SessionItem{
		{
			Record:      storage.SessionRecord{SessionID: "sess_launch", ProductType: "claude_code", CWD: "/tmp"},
			DisplayName: "repo-launch",
			IsLocalNode: true,
			HasValidCWD: true,
		},
	}

	mockSvc := &mockSessionService{items: items}
	app := NewAppModel(mockSvc, true)
	app.loadSessions()

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m, cmd := app.Update(enterMsg)
	app = m.(*AppModel)

	if app.launchAction == nil {
		t.Fatalf("expected launchAction to be set on Enter")
	}
	if app.launchAction.Record.SessionID != "sess_launch" {
		t.Errorf("expected launchAction for sess_launch, got %s", app.launchAction.Record.SessionID)
	}
	if cmd == nil {
		t.Errorf("expected tea.Quit command on launch")
	}
}

func TestAppModel_View(t *testing.T) {
	items := []session.SessionItem{
		{
			Record:      storage.SessionRecord{SessionID: "sess_view", ProductType: "claude_code", CWD: "/tmp"},
			DisplayName: "repo-view",
			IsLocalNode: true,
			HasValidCWD: true,
		},
	}

	mockSvc := &mockSessionService{items: items}
	app := NewAppModel(mockSvc, true)
	app.termWidth = 100
	app.termHeight = 30
	app.loadSessions()

	// 1. Inactive search View
	viewInactive := app.View()
	if viewInactive == "" {
		t.Errorf("expected non-empty view when search is inactive")
	}

	// 2. Active search View (empty query, placeholder active)
	slashMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	m, _ := app.Update(slashMsg)
	app = m.(*AppModel)

	if !app.searchActive {
		t.Fatalf("expected searchActive to be true after '/'")
	}
	viewActive := app.View()
	if viewActive == "" {
		t.Errorf("expected non-empty view when search is active")
	}

	// 3. Small terminal View
	app.termWidth = 10
	app.termHeight = 5
	viewSmall := app.View()
	if viewSmall == "" {
		t.Errorf("expected small terminal warning message")
	}
}
