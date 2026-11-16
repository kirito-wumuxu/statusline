package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/storage"
)

func TestTranscriptScannerSync(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 预置一个 session
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	sessID := "sync-test-session"
	rawJSON := `{"session_id":"` + sessID + `","cwd":"/tmp","model":{"id":"gemini-3.8-flash-high"},"transcript_path":"` + logPath + `"}`
	env, _, _ := model.ParseAndNormalize([]byte(rawJSON), node.NodeInfo{NodeID: "node_1"})
	if err := store.Ingest(env); err != nil {
		t.Fatal(err)
	}

	// 写入 2 个 step
	lines := `{"step_index": 1, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:00:00Z", "input_tokens": 1000, "output_tokens": 50, "cache_read_tokens": 20000}` + "\n" +
		`{"step_index": 2, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:01:00Z", "input_tokens": 1200, "output_tokens": 60, "cache_read_tokens": 25000}` + "\n"
	if err := os.WriteFile(logPath, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewTranscriptScanner(store)
	scanner.SetHomeDir("")
	res, err := scanner.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if res.NewEvents != 2 {
		t.Fatalf("expected 2 new events, got %d", res.NewEvents)
	}

	// 再次同步，幂等测试
	res2, err := scanner.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("sync 2 failed: %v", err)
	}
	if res2.NewEvents != 0 {
		t.Fatalf("expected 0 new events on repeat sync, got %d", res2.NewEvents)
	}
}

func TestHandleSyncTranscriptsAPI(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_api.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	srv := NewServer(&config.ServerConfig{}, store, nil)

	// Send POST /api/v1/sync/transcripts
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/sync/transcripts", nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var res SyncResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
}
