package ui

import (
	"strings"
	"testing"

	"statusline/internal/session"
	"statusline/internal/storage"
)

func TestRenderSessionList(t *testing.T) {
	items := []session.SessionItem{
		{
			Record: storage.SessionRecord{
				SessionID:   "sess_01",
				ProductType: "antigravity",
				GitRepoName: "statusline",
				GitBranch:   "main",
				LastActiveAt: "2026-11-18T00:00:00Z",
			},
			DisplayName:  "statusline",
			RelativeTime: "5m 前",
			IsLocalNode:  true,
			HasValidCWD:  true,
		},
		{
			Record: storage.SessionRecord{
				SessionID:   "sess_02",
				ProductType: "claude_code",
				GitRepoName: "web-ui",
				GitBranch:   "feat/auth",
				LastActiveAt: "2026-11-18T00:00:00Z",
			},
			DisplayName:  "web-ui",
			RelativeTime: "1h 前",
			IsLocalNode:  true,
			HasValidCWD:  false,
		},
	}

	rendered := RenderSessionList(items, 0, 45, 10, true)
	if !strings.Contains(rendered, "statusline") {
		t.Errorf("expected statusline in list, got: %s", rendered)
	}
	if !strings.Contains(rendered, "[AGY]") && !strings.Contains(rendered, "AGY") {
		t.Errorf("expected AGY badge, got: %s", rendered)
	}
	if !strings.Contains(rendered, ">") {
		t.Errorf("expected cursor pointer > in active item, got: %s", rendered)
	}
}

func TestRenderDetailPane(t *testing.T) {
	item := &session.SessionItem{
		Record: storage.SessionRecord{
			SessionID:      "sess_test_99",
			ProductType:    "claude_code",
			CWD:            "/tmp/test-project",
			GitBranch:      "dev",
			LatestModelID:  "claude-3-7-sonnet",
			TotalTokens:    125000,
			TotalInputTokens: 100000,
			TotalOutputTokens: 25000,
		},
		DisplayName:  "test-project",
		RelativeTime: "刚刚",
		IsLocalNode:  true,
		HasValidCWD:  true,
	}

	rendered := RenderDetailPane(item, 50)
	if !strings.Contains(rendered, "sess_test_99") {
		t.Errorf("expected session ID in details, got: %s", rendered)
	}
	if !strings.Contains(rendered, "/tmp/test-project") {
		t.Errorf("expected CWD in details, got: %s", rendered)
	}
}

func TestRenderPreviewPane(t *testing.T) {
	prompt := "How does Bubble Tea work?"
	reply := "Bubble Tea uses the Elm architecture."

	rendered := RenderPreviewPane(prompt, reply, 50, 10)
	if !strings.Contains(rendered, "How does Bubble Tea work?") {
		t.Errorf("expected prompt in preview, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Elm architecture") {
		t.Errorf("expected reply in preview, got: %s", rendered)
	}

	// Empty preview test
	emptyRendered := RenderPreviewPane("", "", 50, 10)
	if !strings.Contains(emptyRendered, "暂无") {
		t.Errorf("expected empty notice, got: %s", emptyRendered)
	}
}
