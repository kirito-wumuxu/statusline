package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/model"
	"statusline/internal/storage"
)

func setupTestServer(t *testing.T) (*Server, *storage.Store) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "server_test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	cfg := config.DefaultServerConfig()
	srv := NewServer(cfg, store, nil)
	return srv, store
}

func TestHandlersReportAndQuery(t *testing.T) {
	srv, store := setupTestServer(t)
	defer store.Close()

	// 1. Test POST /api/v1/report
	payload := `{"product":"antigravity","session_id":"sess_test_1","cwd":"/test","model":{"id":"gemini-flash"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)
	t.Logf("report response: code=%d, body=%s", w.Code, w.Body.String())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from report, got %d (body: %s)", w.Code, w.Body.String())
	}

	// 2. Test GET /api/v1/overview
	reqOverview := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	wOverview := httptest.NewRecorder()
	srv.Router().ServeHTTP(wOverview, reqOverview)
	if wOverview.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from overview, got %d", wOverview.Code)
	}
	var ovMap map[string]interface{}
	if err := json.Unmarshal(wOverview.Body.Bytes(), &ovMap); err != nil {
		t.Fatalf("failed to unmarshal overview map: %v", err)
	}
	// IMP-3: Verify metrics array is present for Web Dashboard SVG trend chart
	metrics, ok := ovMap["metrics"].([]interface{})
	if !ok || len(metrics) == 0 {
		t.Fatalf("expected non-empty metrics array for trend chart, got %+v", ovMap["metrics"])
	}

	// 3. Test GET /api/v1/sessions
	reqSess := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	wSess := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSess, reqSess)
	if wSess.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from sessions, got %d", wSess.Code)
	}

	// 4. Test GET /api/v1/nodes
	reqNodes := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	wNodes := httptest.NewRecorder()
	srv.Router().ServeHTTP(wNodes, reqNodes)
	if wNodes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from nodes, got %d", wNodes.Code)
	}

	// 5. Test GET /api/v1/history
	reqHist := httptest.NewRequest(http.MethodGet, "/api/v1/history?session_id=sess_test_1", nil)
	wHist := httptest.NewRecorder()
	srv.Router().ServeHTTP(wHist, reqHist)
	if wHist.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from history, got %d", wHist.Code)
	}

	// 6. Test GET /api/v1/tokens/daily
	reqDaily := httptest.NewRequest(http.MethodGet, "/api/v1/tokens/daily", nil)
	wDaily := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDaily, reqDaily)
	if wDaily.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from tokens/daily, got %d", wDaily.Code)
	}

	// 7. Test GET /api/v1/debug/raw
	reqDebug := httptest.NewRequest(http.MethodGet, "/api/v1/debug/raw?limit=5", nil)
	wDebug := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDebug, reqDebug)
	if wDebug.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from debug/raw, got %d", wDebug.Code)
	}
	var debugReports []map[string]interface{}
	if err := json.Unmarshal(wDebug.Body.Bytes(), &debugReports); err != nil {
		t.Fatalf("failed to unmarshal debug raw reports: %v", err)
	}
	if len(debugReports) == 0 {
		t.Fatalf("expected at least 1 raw report event in debug API, got 0")
	}
}

func TestHandlerReportEnvelope(t *testing.T) {
	srv, store := setupTestServer(t)
	defer store.Close()

	env := model.TelemetryEnvelope{
		NodeID: "node_envelope_test",
		Normalized: model.NormalizedData{
			SessionID:   "sess_env_1",
			ProductType: "antigravity",
			ModelID:     "gemini-pro",
		},
	}
	body, _ := json.Marshal(env)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)
	t.Logf("report response: code=%d, body=%s", w.Code, w.Body.String())
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}


func TestHandlerManualRefresh(t *testing.T) {
	// IMP-2: Web dashboard Manual Refresh button sends POST /api/refresh
	srv, store := setupTestServer(t)
	defer store.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	w := httptest.NewRecorder()

	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /api/refresh, got %d (body: %s)", w.Code, w.Body.String())
	}
}
