package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/storage"
)

func TestHandleSyncEventsAPI(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_sync_api.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.ServerConfig{
		Server: config.ServerListenConfig{
			APIKey: "secret-test-key",
		},
	}
	srv := NewServer(cfg, store, nil)

	batch := storage.SyncBatch{
		NodeID: "node_api_sync",
		Events: []storage.SyncEventItem{
			{
				SessionID:           "session_api_1",
				PromptID:            "step_1",
				ProductType:         "antigravity",
				ReportedAt:          "2026-11-16T10:00:00Z",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1000,
				CurrentOutputTokens: 200,
				CacheReadTokens:     500,
			},
			{
				SessionID:           "session_api_1",
				PromptID:            "step_2",
				ProductType:         "antigravity",
				ReportedAt:          "2026-11-16T10:01:00Z",
				ModelID:             "gemini-flash",
				CurrentInputTokens:  1200,
				CurrentOutputTokens: 250,
				CacheReadTokens:     600,
			},
		},
	}

	bodyBytes, _ := json.Marshal(batch)

	// 1. Unauthorized request without API key
	reqUnauth, _ := http.NewRequest(http.MethodPost, "/api/v1/sync/events", bytes.NewReader(bodyBytes))
	rrUnauth := httptest.NewRecorder()
	srv.Router().ServeHTTP(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rrUnauth.Code)
	}

	// 2. Authorized request
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/sync/events", bytes.NewReader(bodyBytes))
	req.Header.Set("X-API-Key", "secret-test-key")
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" || int(resp["inserted"].(float64)) != 2 || int(resp["skipped"].(float64)) != 0 {
		t.Fatalf("unexpected resp 1: %+v", resp)
	}

	// 3. Repeat request (Idempotency / Deduplication)
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/sync/events", bytes.NewReader(bodyBytes))
	req2.Header.Set("X-API-Key", "secret-test-key")
	rr2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr2.Code)
	}

	var resp2 map[string]interface{}
	if err := json.Unmarshal(rr2.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}
	if int(resp2["inserted"].(float64)) != 0 || int(resp2["skipped"].(float64)) != 2 {
		t.Fatalf("unexpected resp 2 (expected 0 inserted, 2 skipped): %+v", resp2)
	}
}
