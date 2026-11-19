package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
		"context_window": {
			"current_usage": {"input_tokens": 2500, "output_tokens": 800},
			"total_input_tokens": 2500, 
			"total_output_tokens": 800, 
			"context_window_size": 1048576
		}
	}`), n)
	env2.Normalized.PromptID = "step_1"

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

func TestMultiDayTokenConsumptionDelta(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "delta_tokens_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	stepIdx := 0
	// Helper to ingest event with custom timestamp
	ingestEventWithTime := func(sessionID, modelID string, curIn, curOut int64, timeStr string) {
		stepIdx++
		env := &model.TelemetryEnvelope{
			NodeID:     "node_delta",
			ReportedAt: timeStr,
			Normalized: model.NormalizedData{
				ProductType:         "antigravity",
				SessionID:           sessionID,
				ModelID:             modelID,
				ModelDisplayName:    modelID,
				PromptID:            fmt.Sprintf("step_%d", stepIdx),
				CurrentInputTokens:  curIn,
				CurrentOutputTokens: curOut,
			},
		}
		if err := store.Ingest(env); err != nil {
			t.Fatalf("failed to ingest: %v", err)
		}
	}

	// Day 1 (yesterday): 2 turns, total 2000 in, 1200 out
	tYesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z")
	tYesterdayDate := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	ingestEventWithTime("sess-delta-1", "gemini-3.8-flash", 1000, 500, tYesterday)
	ingestEventWithTime("sess-delta-1", "gemini-3.8-flash", 1000, 700, tYesterday)

	// Day 2 (today): 1 turn with 500 in, 400 out
	tToday := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	tTodayDate := time.Now().UTC().Format("2006-01-02")
	ingestEventWithTime("sess-delta-1", "gemini-3.8-flash", 500, 400, tToday)

	// Ingest an idle event with 0 turn tokens and empty prompt_id (should not insert junk event)
	idleEnv := &model.TelemetryEnvelope{
		NodeID:     "node_delta",
		ReportedAt: tToday,
		Normalized: model.NormalizedData{
			ProductType:      "antigravity",
			SessionID:        "sess-delta-1",
			ModelID:          "gemini-3.8-flash",
			ModelDisplayName: "gemini-3.8-flash",
		},
	}
	if err := store.Ingest(idleEnv); err != nil {
		t.Fatalf("failed to ingest idle: %v", err)
	}

	// Query GetDailyModelTokens
	daily, err := store.GetDailyModelTokens(7)
	if err != nil {
		t.Fatalf("failed to get daily tokens: %v", err)
	}

	var todaySummary, yesterdaySummary *DailyTokensSummary
	for i := range daily {
		if daily[i].Date == tTodayDate {
			todaySummary = &daily[i]
		}
		if daily[i].Date == tYesterdayDate {
			yesterdaySummary = &daily[i]
		}
	}

	if yesterdaySummary == nil {
		t.Fatalf("expected summary for yesterday %s, got none", tYesterdayDate)
	}
	if yesterdaySummary.TotalInputTokens != 2000 || yesterdaySummary.TotalOutputTokens != 1200 || yesterdaySummary.TotalTokens != 3200 {
		t.Errorf("yesterday expected 2000 in, 1200 out, 3200 total; got in=%d, out=%d, total=%d",
			yesterdaySummary.TotalInputTokens, yesterdaySummary.TotalOutputTokens, yesterdaySummary.TotalTokens)
	}

	if todaySummary == nil {
		t.Fatalf("expected summary for today %s, got none", tTodayDate)
	}
	// Today should ONLY count the delta (+500 in, +400 out = 900 total)!
	if todaySummary.TotalInputTokens != 500 || todaySummary.TotalOutputTokens != 400 || todaySummary.TotalTokens != 900 {
		t.Errorf("today expected delta 500 in, 400 out, 900 total; got in=%d, out=%d, total=%d",
			todaySummary.TotalInputTokens, todaySummary.TotalOutputTokens, todaySummary.TotalTokens)
	}

	// Total across yesterday + today should equal the session's total (2500 in + 1600 out = 4100)!
	sumAcrossDays := yesterdaySummary.TotalTokens + todaySummary.TotalTokens
	if sumAcrossDays != 4100 {
		t.Errorf("expected sum across days to be 4100, got %d", sumAcrossDays)
	}
}

func TestMultiModelSessionTokenDelta(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "multimodel_delta_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	stepCount := 0
	ingestEvent := func(sessionID, modelID, displayName string, inTokens, outTokens int64, timeStr string) {
		stepCount++
		env := &model.TelemetryEnvelope{
			NodeID:     "node_multimodel",
			ReportedAt: timeStr,
			Normalized: model.NormalizedData{
				ProductType:         "antigravity",
				SessionID:           sessionID,
				ModelID:             modelID,
				ModelDisplayName:    displayName,
				PromptID:            fmt.Sprintf("step_%d", stepCount),
				CurrentInputTokens:  inTokens,
				CurrentOutputTokens: outTokens,
			},
		}
		if err := store.Ingest(env); err != nil {
			t.Fatalf("failed to ingest: %v", err)
		}
	}

	// Day 1 (yesterday)
	tYesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z")
	tYesterdayDate := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")

	// Session starts with gpt-oss: turn 1 (5000 in, 1000 out), turn 2 (3000 in, 1500 out) -> 8000 in, 2500 out
	ingestEvent("sess-switch-1", "gpt-oss", "GPT-OSS 120B", 5000, 1000, tYesterday)
	ingestEvent("sess-switch-1", "gpt-oss", "GPT-OSS 120B", 3000, 1500, tYesterday)

	// User switches to claude-opus in the same session: turn 3 (4000 in, 1500 out)
	ingestEvent("sess-switch-1", "claude-opus", "Claude Opus", 4000, 1500, tYesterday)

	// User switches to gemini-flash in the same session: turn 4 (3000 in, 2000 out)
	ingestEvent("sess-switch-1", "gemini-flash", "Gemini Flash", 3000, 2000, tYesterday)

	// User switches back to gpt-oss: turn 5 (3000 in, 1500 out) -> gpt-oss total = 11000 in, 4000 out = 15000
	ingestEvent("sess-switch-1", "gpt-oss", "GPT-OSS 120B", 3000, 1500, tYesterday)

	// Day 2 (today)
	tToday := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	tTodayDate := time.Now().UTC().Format("2006-01-02")

	// Continues with gemini-flash today: turn 6 (2000 in, 1000 out)
	ingestEvent("sess-switch-1", "gemini-flash", "Gemini Flash", 2000, 1000, tToday)
	// Switches to claude-opus today: turn 7 (3000 in, 1000 out)
	ingestEvent("sess-switch-1", "claude-opus", "Claude Opus", 3000, 1000, tToday)

	// Query daily model tokens
	daily, err := store.GetDailyModelTokens(7)
	if err != nil {
		t.Fatalf("failed to get daily tokens: %v", err)
	}

	var day1, day2 *DailyTokensSummary
	for i := range daily {
		if daily[i].Date == tYesterdayDate {
			day1 = &daily[i]
		}
		if daily[i].Date == tTodayDate {
			day2 = &daily[i]
		}
	}

	if day1 == nil {
		t.Fatalf("expected day 1 summary for %s, got none", tYesterdayDate)
	}
	if day2 == nil {
		t.Fatalf("expected day 2 summary for %s, got none", tTodayDate)
	}

	// Verify Day 1:
	// Total input should be 18000, total output 7500, total tokens 25500
	if day1.TotalInputTokens != 18000 || day1.TotalOutputTokens != 7500 || day1.TotalTokens != 25500 {
		t.Errorf("day 1 totals expected in=18000, out=7500, total=25500; got in=%d, out=%d, total=%d",
			day1.TotalInputTokens, day1.TotalOutputTokens, day1.TotalTokens)
	}

	// Model breakdown on Day 1:
	// gpt-oss: in=5000 + 3000(8000-5000) + 3000(18000-15000) = 11000; out=1000 + 1500(2500-1000) + 1500(7500-6000) = 4000; total = 15000
	// claude-opus: in=4000(12000-8000); out=1500(4000-2500); total = 5500
	// gemini-flash: in=3000(15000-12000); out=2000(6000-4000); total = 5000
	modelTokensDay1 := make(map[string]DailyModelToken)
	var sumDay1ModelTokens int64
	for _, m := range day1.Models {
		modelTokensDay1[m.ModelID] = m
		sumDay1ModelTokens += m.TotalTokens
	}

	if sumDay1ModelTokens != 25500 {
		t.Errorf("sum of model tokens on day 1 should be 25500, got %d", sumDay1ModelTokens)
	}

	gpt := modelTokensDay1["gpt-oss"]
	if gpt.InputTokens != 11000 || gpt.OutputTokens != 4000 || gpt.TotalTokens != 15000 {
		t.Errorf("gpt-oss expected in=11000, out=4000, total=15000; got in=%d, out=%d, total=%d",
			gpt.InputTokens, gpt.OutputTokens, gpt.TotalTokens)
	}

	claude := modelTokensDay1["claude-opus"]
	if claude.InputTokens != 4000 || claude.OutputTokens != 1500 || claude.TotalTokens != 5500 {
		t.Errorf("claude-opus expected in=4000, out=1500, total=5500; got in=%d, out=%d, total=%d",
			claude.InputTokens, claude.OutputTokens, claude.TotalTokens)
	}

	gemini := modelTokensDay1["gemini-flash"]
	if gemini.InputTokens != 3000 || gemini.OutputTokens != 2000 || gemini.TotalTokens != 5000 {
		t.Errorf("gemini-flash expected in=3000, out=2000, total=5000; got in=%d, out=%d, total=%d",
			gemini.InputTokens, gemini.OutputTokens, gemini.TotalTokens)
	}

	// Verify Day 2:
	// Day 2 delta: in = 23000 - 18000 = 5000; out = 9500 - 7500 = 2000; total = 7000
	if day2.TotalInputTokens != 5000 || day2.TotalOutputTokens != 2000 || day2.TotalTokens != 7000 {
		t.Errorf("day 2 totals expected in=5000, out=2000, total=7000; got in=%d, out=%d, total=%d",
			day2.TotalInputTokens, day2.TotalOutputTokens, day2.TotalTokens)
	}

	// Model breakdown on Day 2:
	// gemini-flash: in=2000(20000-18000); out=1000(8500-7500); total = 3000
	// claude-opus: in=3000(23000-20000); out=1000(9500-8500); total = 4000
	modelTokensDay2 := make(map[string]DailyModelToken)
	var sumDay2ModelTokens int64
	for _, m := range day2.Models {
		modelTokensDay2[m.ModelID] = m
		sumDay2ModelTokens += m.TotalTokens
	}

	if sumDay2ModelTokens != 7000 {
		t.Errorf("sum of model tokens on day 2 should be 7000, got %d", sumDay2ModelTokens)
	}

	geminiD2 := modelTokensDay2["gemini-flash"]
	if geminiD2.InputTokens != 2000 || geminiD2.OutputTokens != 1000 || geminiD2.TotalTokens != 3000 {
		t.Errorf("day 2 gemini-flash expected in=2000, out=1000, total=3000; got in=%d, out=%d, total=%d",
			geminiD2.InputTokens, geminiD2.OutputTokens, geminiD2.TotalTokens)
	}

	claudeD2 := modelTokensDay2["claude-opus"]
	if claudeD2.InputTokens != 3000 || claudeD2.OutputTokens != 1000 || claudeD2.TotalTokens != 4000 {
		t.Errorf("day 2 claude-opus expected in=3000, out=1000, total=4000; got in=%d, out=%d, total=%d",
			claudeD2.InputTokens, claudeD2.OutputTokens, claudeD2.TotalTokens)
	}

	// Strict Conservation Check:
	// Total across all days and models must strictly equal the latest session total: 23000 in + 9500 out = 32500 total!
	grandTotal := day1.TotalTokens + day2.TotalTokens
	if grandTotal != 32500 {
		t.Errorf("grand total tokens across all days and models must be 32500, got %d", grandTotal)
	}
}

func TestPriorityAliasesStorage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "priority_test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// 1. Verify that InitDB auto-seeded the 7 default priorities
	initialCfg, err := store.GetDisplayAliases()
	if err != nil {
		t.Fatalf("GetDisplayAliases failed: %v", err)
	}
	if len(initialCfg.Priorities) != 7 {
		t.Fatalf("expected 7 default priorities auto-seeded, got %d", len(initialCfg.Priorities))
	}
	if initialCfg.Priorities[0].Key != "model" || initialCfg.Priorities[0].SortOrder != 1 {
		t.Errorf("expected default #1 priority to be model, got %+v", initialCfg.Priorities[0])
	}
	if initialCfg.Priorities[6].Key != "dir" || initialCfg.Priorities[6].SortOrder != 7 {
		t.Errorf("expected default #7 priority to be dir, got %+v", initialCfg.Priorities[6])
	}

	// 2. Custom update: invert priority (dir = 1, model = 7)
	customPriorities := []model.DisplayAliasItem{
		{Category: "priority", Key: "dir", DisplayName: "工作区目录", SortOrder: 1},
		{Category: "priority", Key: "model", DisplayName: "模型标识", SortOrder: 7},
	}

	if err := store.SaveDisplayAliases(customPriorities); err != nil {
		t.Fatalf("failed to save custom priority aliases: %v", err)
	}

	cfg, err := store.GetDisplayAliases()
	if err != nil {
		t.Fatalf("failed to get display aliases: %v", err)
	}

	if len(cfg.Priorities) != 7 {
		t.Fatalf("expected 7 priorities, got %d", len(cfg.Priorities))
	}
	if cfg.Priorities[0].Key != "dir" || cfg.Priorities[0].SortOrder != 1 {
		t.Errorf("expected first priority to now be dir with sort_order 1, got %+v", cfg.Priorities[0])
	}
	if cfg.Priorities[6].Key != "model" || cfg.Priorities[6].SortOrder != 7 {
		t.Errorf("expected last priority to now be model with sort_order 7, got %+v", cfg.Priorities[6])
	}
}


func TestDailyModelTokensAtomicAggregation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_atomic.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	nowStr := time.Now().UTC().Format(time.RFC3339)
	sessID := "atomic-sess-1"
	e1 := &model.TelemetryEnvelope{
		ReportedAt: nowStr,
		Payload:    map[string]interface{}{},
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "gemini-3.8-flash-high",
			ModelDisplayName:    "Gemini 3.8 Flash (High)",
			CurrentInputTokens:  4000,
			CurrentOutputTokens: 100,
			CacheReadTokens:     60000,
		},
	}
	e2 := &model.TelemetryEnvelope{
		ReportedAt: nowStr,
		Payload:    map[string]interface{}{},
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "claude-opus-4-6",
			ModelDisplayName:    "Claude Opus 4.6",
			CurrentInputTokens:  2000,
			CurrentOutputTokens: 50,
			CacheReadTokens:     15000,
		},
	}
	if err := s.Ingest(e1); err != nil {
		t.Fatal(err)
	}
	if err := s.Ingest(e2); err != nil {
		t.Fatal(err)
	}

	summaries, err := s.GetDailyModelTokens(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	sum := summaries[0]
	// 4000 + 100 + 60000 + 2000 + 50 + 15000 = 81150
	expectedTotal := int64(81150)
	if sum.TotalTokens != expectedTotal {
		t.Fatalf("expected total tokens %d, got %d", expectedTotal, sum.TotalTokens)
	}
	if sum.TotalCacheReadTokens != 75000 {
		t.Fatalf("expected cache read tokens 75000, got %d", sum.TotalCacheReadTokens)
	}
}

func TestIngestPromptIDDeduplication(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_dedup.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	sessID := "sess-dedup-test"
	env1 := &model.TelemetryEnvelope{
		ReportedAt: "2026-11-16T10:00:00Z",
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "gemini-flash",
			PromptID:            "step_1",
			CurrentInputTokens:  1000,
			CurrentOutputTokens: 200,
		},
	}
	if err := store.Ingest(env1); err != nil {
		t.Fatal(err)
	}

	// Ingest exact duplicate with same (session_id, prompt_id)
	env2 := &model.TelemetryEnvelope{
		ReportedAt: "2026-11-16T10:00:05Z",
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "gemini-flash",
			PromptID:            "step_1",
			CurrentInputTokens:  1000,
			CurrentOutputTokens: 200,
		},
	}
	if err := store.Ingest(env2); err != nil {
		t.Fatal(err)
	}

	// Verify telemetry_events contains only 1 row!
	var count int
	if err := store.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events WHERE session_id = ?", sessID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event after duplicate ingest, got %d", count)
	}
}

func TestIngestSyncBatchDeduplication(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_sync_batch.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	batch1 := &SyncBatch{
		NodeID: "node_sync_test",
		Events: []SyncEventItem{
			{
				SessionID:           "sess_sync_1",
				NodeID:              "node_sync_test",
				PromptID:            "step_1",
				ProductType:         "antigravity",
				ReportedAt:          "2026-11-16T08:00:00Z",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1000,
				CurrentOutputTokens: 200,
				CacheReadTokens:     500,
			},
			{
				SessionID:           "sess_sync_1",
				NodeID:              "node_sync_test",
				PromptID:            "step_2",
				ProductType:         "antigravity",
				ReportedAt:          "2026-11-16T08:01:00Z",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1200,
				CurrentOutputTokens: 250,
				CacheReadTokens:     800,
			},
			{
				SessionID:           "sess_sync_2",
				NodeID:              "node_sync_test",
				PromptID:            "msg_01",
				ProductType:         "claude_code",
				ReportedAt:          "2026-11-16T08:02:00Z",
				ModelID:             "claude-3-7-sonnet",
				CurrentInputTokens:  2000,
				CurrentOutputTokens: 300,
				CacheReadTokens:     1500,
			},
		},
	}

	// 1st Ingestion: all 3 should be inserted
	res1, err := store.IngestSyncBatch(ctx, batch1)
	if err != nil {
		t.Fatalf("first IngestSyncBatch failed: %v", err)
	}
	if res1.Received != 3 || res1.Inserted != 3 || res1.Skipped != 0 {
		t.Fatalf("unexpected res1: %+v", res1)
	}

	// 2nd Ingestion: duplicate step_1 and msg_01, but new step_3
	batch2 := &SyncBatch{
		NodeID: "node_sync_test",
		Events: []SyncEventItem{
			{
				SessionID:           "sess_sync_1",
				PromptID:            "step_1", // duplicate
				ProductType:         "antigravity",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1000,
				CurrentOutputTokens: 200,
			},
			{
				SessionID:           "sess_sync_2",
				PromptID:            "msg_01", // duplicate
				ProductType:         "claude_code",
				ModelID:             "claude-3-7-sonnet",
				CurrentInputTokens:  2000,
				CurrentOutputTokens: 300,
			},
			{
				SessionID:           "sess_sync_1",
				PromptID:            "step_3", // NEW
				ProductType:         "antigravity",
				ReportedAt:          "2026-11-16T08:03:00Z",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1500,
				CurrentOutputTokens: 300,
				CacheReadTokens:     1000,
			},
		},
	}

	res2, err := store.IngestSyncBatch(ctx, batch2)
	if err != nil {
		t.Fatalf("second IngestSyncBatch failed: %v", err)
	}
	if res2.Received != 3 || res2.Inserted != 1 || res2.Skipped != 2 {
		t.Fatalf("unexpected res2: %+v (expected Inserted: 1, Skipped: 2)", res2)
	}

	// Verify total count in DB is 4
	var totalEvents int
	if err := store.DB().QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&totalEvents); err != nil {
		t.Fatal(err)
	}
	if totalEvents != 4 {
		t.Fatalf("expected 4 total events in DB, got %d", totalEvents)
	}
}
