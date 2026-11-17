package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"statusline/internal/storage"
)

type mockStore struct {
	sessions []storage.SessionRecord
}

func (m *mockStore) GetSessions() ([]storage.SessionRecord, error) {
	return m.sessions, nil
}

func TestSessionService_ListSessions_NodeFilter(t *testing.T) {
	tmpDir := t.TempDir()
	validDir := filepath.Join(tmpDir, "repo-a")
	_ = os.MkdirAll(validDir, 0755)

	localNodeID := "node_local_123"
	foreignNodeID := "node_foreign_456"

	records := []storage.SessionRecord{
		{
			SessionID:    "sess_local_1",
			NodeID:       localNodeID,
			ProductType:  "antigravity",
			GitRepoName:  "repo-local",
			CWD:          validDir,
			LastActiveAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		},
		{
			SessionID:    "sess_foreign_2",
			NodeID:       foreignNodeID,
			ProductType:  "claude_code",
			GitRepoName:  "repo-foreign",
			CWD:          "/non/existent/path/on/this/machine",
			LastActiveAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
		},
	}

	svc := NewSessionServiceWithRecords(localNodeID, records)

	// 1. Test onlyLocalNode = true (Default)
	localItems, err := svc.ListSessions(context.Background(), true)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(localItems) != 1 {
		t.Fatalf("expected 1 local session, got %d", len(localItems))
	}
	if localItems[0].Record.SessionID != "sess_local_1" {
		t.Errorf("expected sess_local_1, got %s", localItems[0].Record.SessionID)
	}
	if !localItems[0].IsLocalNode {
		t.Errorf("expected IsLocalNode to be true")
	}
	if !localItems[0].HasValidCWD {
		t.Errorf("expected HasValidCWD to be true for existing dir")
	}

	// 2. Test onlyLocalNode = false (All Nodes)
	allItems, err := svc.ListSessions(context.Background(), false)
	if err != nil {
		t.Fatalf("ListSessions(all) failed: %v", err)
	}
	if len(allItems) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(allItems))
	}

	// Check foreign item has IsLocalNode=false and HasValidCWD=false
	var foundForeign bool
	for _, it := range allItems {
		if it.Record.SessionID == "sess_foreign_2" {
			foundForeign = true
			if it.IsLocalNode {
				t.Errorf("expected foreign item IsLocalNode=false")
			}
			if it.HasValidCWD {
				t.Errorf("expected non-existent CWD HasValidCWD=false")
			}
		}
	}
	if !foundForeign {
		t.Errorf("expected sess_foreign_2 in allItems")
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	cases := []struct {
		diff     time.Duration
		expected string
	}{
		{20 * time.Second, "刚刚"},
		{5 * time.Minute, "5m 前"},
		{3 * time.Hour, "3h 前"},
		{26 * time.Hour, "昨天"},
		{3 * 24 * time.Hour, "3d 前"},
	}

	for _, c := range cases {
		ts := now.Add(-c.diff).Format(time.RFC3339)
		got := FormatRelativeTime(ts)
		if got != c.expected {
			t.Errorf("for diff %v: expected %q, got %q", c.diff, c.expected, got)
		}
	}
}
