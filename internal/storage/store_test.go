package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"statusline/internal/model"
	"statusline/internal/node"
)

func TestStorageLifecycleAndIngest(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	n := node.GetNodeInfo("node_test123", "1.0.0")
	env1, _, err := model.ParseAndNormalize([]byte(`{
		"product": "antigravity",
		"session_id": "sess-agy-001",
		"email": "dev@example.com",
		"cwd": "/workspace/statusline",
		"model": {"id": "gemini-3.8-flash", "display_name": "Gemini 3.8 Flash"},
		"context_window": {"total_input_tokens": 1000, "total_output_tokens": 500, "context_window_size": 1048576},
		"quota": {
			"gemini-weekly": {"remaining_fraction": 0.88, "reset_in_seconds": 3600, "reset_time": "2026-11-12T00:00:00Z"}
		}
	}`), n)
	if err != nil {
		t.Fatalf("failed to parse env1: %v", err)
	}

	if err := store.Ingest(env1); err != nil {
		t.Fatalf("failed to ingest env1: %v", err)
	}

	// Ingest second event for same session with updated token counts (testing UPSERT)
	env2, _, _ := model.ParseAndNormalize([]byte(`{
		"product": "antigravity",
		"session_id": "sess-agy-001",
		"email": "dev@example.com",
		"cwd": "/workspace/statusline",
		"model": {"id": "gemini-3.8-flash", "display_name": "Gemini 3.8 Flash"},
		"context_window": {"total_input_tokens": 2500, "total_output_tokens": 800, "context_window_size": 1048576}
	}`), n)

	if err := store.Ingest(env2); err != nil {
		t.Fatalf("failed to ingest env2: %v", err)
	}

	// Query nodes
	nodes, err := store.GetNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d (err: %v)", len(nodes), err)
	}
	if nodes[0].NodeID != "node_test123" {
		t.Errorf("expected node_test123, got %s", nodes[0].NodeID)
	}

	// Query sessions
	sessions, err := store.GetSessions("", 10)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d (err: %v)", len(sessions), err)
	}
	if sessions[0].TotalInputTokens != 2500 {
		t.Errorf("expected upserted input tokens 2500, got %d", sessions[0].TotalInputTokens)
	}

	// Query history (should have 2 events)
	history, err := store.GetHistory("sess-agy-001", 10)
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 history events, got %d (err: %v)", len(history), err)
	}

	// Query quotas
	quotas, err := store.GetAccountQuotas()
	if err != nil || len(quotas) != 1 {
		t.Fatalf("expected 1 quota record, got %d (err: %v)", len(quotas), err)
	}
	if quotas[0].QuotaKey != "gemini-weekly" {
		t.Errorf("expected quota key gemini-weekly, got %s", quotas[0].QuotaKey)
	}

	// Query overview
	overview, err := store.GetOverview()
	if err != nil {
		t.Fatalf("failed to get overview: %v", err)
	}
	if overview.ActiveNodes != 1 || overview.ActiveSessions != 1 {
		t.Errorf("expected 1 active node and session, got %d / %d", overview.ActiveNodes, overview.ActiveSessions)
	}
	if overview.TotalInputTokens != 2500 {
		t.Errorf("expected 2500 total input tokens, got %d", overview.TotalInputTokens)
	}
}

func TestConcurrentWrites(t *testing.T) {
	// Review Focus #4: Burst concurrent writes must not trigger 'database is locked' in WAL mode
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "concurrent_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	n := node.GetNodeInfo("node_concurrent", "1.0.0")

	concurrency := 50
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := fmt.Sprintf(`{
				"session_id": "sess-concurrent-%d",
				"cwd": "/workspace",
				"model": {"id": "claude-3-7-sonnet"},
				"context_window": {"total_input_tokens": %d, "total_output_tokens": 100}
			}`, idx%10, idx*10)

			env, _, parseErr := model.ParseAndNormalize([]byte(payload), n)
			if parseErr != nil {
				errCh <- parseErr
				return
			}
			if ingestErr := store.Ingest(env); ingestErr != nil {
				errCh <- ingestErr
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent write failed with error: %v", err)
		}
	}

	// Verify events count
	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != concurrency {
		t.Fatalf("expected %d events, got %d", concurrency, count)
	}
}
