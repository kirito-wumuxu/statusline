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

	// Query daily model tokens
	dailyTokens, err := store.GetDailyModelTokens(7)
	if err != nil {
		t.Fatalf("failed to get daily tokens: %v", err)
	}
	if len(dailyTokens) == 0 {
		t.Fatalf("expected daily tokens for today, got 0")
	}
	if dailyTokens[0].TotalInputTokens != 2500 {
		t.Errorf("expected 2500 input tokens today, got %d", dailyTokens[0].TotalInputTokens)
	}
	if len(dailyTokens[0].Models) != 1 || dailyTokens[0].Models[0].ModelID != "gemini-3.8-flash" {
		t.Errorf("expected gemini-3.8-flash model summary, got %+v", dailyTokens[0].Models)
	}

	// Query recent raw reports
	rawReports, err := store.GetRecentRawReports(5)
	if err != nil {
		t.Fatalf("failed to get raw reports: %v", err)
	}
	if len(rawReports) != 2 {
		t.Fatalf("expected 2 raw reports, got %d", len(rawReports))
	}
	if rawReports[0].RawPayload["product"] != "antigravity" {
		t.Errorf("expected product antigravity in raw payload, got %v", rawReports[0].RawPayload["product"])
	}

	// Query grouped account quotas
	groupedQuotas, err := store.GetGroupedAccountQuotas()
	if err != nil {
		t.Fatalf("failed to get grouped account quotas: %v", err)
	}
	if len(groupedQuotas) != 1 || groupedQuotas[0].AccountEmail != "dev@example.com" {
		t.Fatalf("expected 1 grouped quota for dev@example.com, got %+v", groupedQuotas)
	}
	if groupedQuotas[0].Gemini == nil {
		t.Errorf("expected Gemini quota present in group")
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

func TestAccountAliasSortOrder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sort_order_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	n := node.GetNodeInfo("node_sort", "1.0.0")

	// Ingest 3 accounts
	for _, email := range []string{"charlie@example.com", "alice@example.com", "bob@example.com"} {
		payload := fmt.Sprintf(`{
			"product": "antigravity",
			"session_id": "sess-%s",
			"email": "%s",
			"cwd": "/workspace",
			"model": {"id": "gemini-3.8-flash"},
			"quota": {
				"gemini-weekly": {"remaining_fraction": 0.9, "reset_in_seconds": 3600}
			}
		}`, email, email)
		env, _, err := model.ParseAndNormalize([]byte(payload), n)
		if err != nil {
			t.Fatalf("failed to parse: %v", err)
		}
		if err := store.Ingest(env); err != nil {
			t.Fatalf("failed to ingest: %v", err)
		}
	}

	// Configure sort_order: bob = 1, alice = 2, charlie = 0
	aliasesToSave := []model.DisplayAliasItem{
		{Category: "account", Key: "bob@example.com", DisplayName: "Bob", ShortName: "B", SortOrder: 1},
		{Category: "account", Key: "alice@example.com", DisplayName: "Alice", ShortName: "A", SortOrder: 2},
		{Category: "account", Key: "charlie@example.com", DisplayName: "Charlie", ShortName: "C", SortOrder: 0},
	}
	if err := store.SaveDisplayAliases(aliasesToSave); err != nil {
		t.Fatalf("failed to save aliases: %v", err)
	}

	// 1. Verify GetDisplayAliases ordering
	cfg, err := store.GetDisplayAliases()
	if err != nil {
		t.Fatalf("failed to get display aliases: %v", err)
	}
	if len(cfg.Accounts) != 3 {
		t.Fatalf("expected 3 accounts in aliases, got %d", len(cfg.Accounts))
	}
	if cfg.Accounts[0].Key != "bob@example.com" || cfg.Accounts[0].SortOrder != 1 {
		t.Errorf("expected first account bob with sort 1, got %+v", cfg.Accounts[0])
	}
	if cfg.Accounts[1].Key != "alice@example.com" || cfg.Accounts[1].SortOrder != 2 {
		t.Errorf("expected second account alice with sort 2, got %+v", cfg.Accounts[1])
	}
	if cfg.Accounts[2].Key != "charlie@example.com" || cfg.Accounts[2].SortOrder != 0 {
		t.Errorf("expected third account charlie with sort 0, got %+v", cfg.Accounts[2])
	}

	// 2. Verify GetGroupedAccountQuotas ordering
	groups, err := store.GetGroupedAccountQuotas()
	if err != nil {
		t.Fatalf("failed to get grouped quotas: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 grouped quotas, got %d", len(groups))
	}
	if groups[0].AccountEmail != "bob@example.com" {
		t.Errorf("expected #1 group to be bob@example.com, got %s", groups[0].AccountEmail)
	}
	if groups[1].AccountEmail != "alice@example.com" {
		t.Errorf("expected #2 group to be alice@example.com, got %s", groups[1].AccountEmail)
	}
	if groups[2].AccountEmail != "charlie@example.com" {
		t.Errorf("expected #3 group to be charlie@example.com, got %s", groups[2].AccountEmail)
	}
}
