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

	// 8. Test GET /api/v1/config/aliases
	reqAliases := httptest.NewRequest(http.MethodGet, "/api/v1/config/aliases", nil)
	wAliases := httptest.NewRecorder()
	srv.Router().ServeHTTP(wAliases, reqAliases)
	if wAliases.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from config/aliases, got %d", wAliases.Code)
	}

	var aliases model.DisplayAliasesConfig
	if err := json.Unmarshal(wAliases.Body.Bytes(), &aliases); err != nil {
		t.Fatalf("failed to decode aliases json: %v", err)
	}
	if len(aliases.Quotas) == 0 {
		t.Errorf("expected default quota aliases, got 0")
	}

	// 9. Test POST /api/v1/config/aliases
	postPayload := `[{"category":"quota","key":"test-key","name":"Test Model","short_name":"TM"}]`
	reqPostAliases := httptest.NewRequest(http.MethodPost, "/api/v1/config/aliases", bytes.NewBufferString(postPayload))
	reqPostAliases.Header.Set("Content-Type", "application/json")
	wPostAliases := httptest.NewRecorder()
	srv.Router().ServeHTTP(wPostAliases, reqPostAliases)
	if wPostAliases.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from POST config/aliases, got %d", wPostAliases.Code)
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

func TestHandlerReportAPIKeyAuth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "server_auth_test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultServerConfig()
	cfg.Server.APIKey = "secret-token-123"
	srv := NewServer(cfg, store, nil)

	payload := `{"product":"antigravity","session_id":"sess_auth","model":{"id":"gemini-flash"}}`

	// 1. Without auth header -> 401 Unauthorized
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w1.Code)
	}

	// 2. With wrong auth header -> 401 Unauthorized
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-API-Key", "wrong-key")
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong key, got %d", w2.Code)
	}

	// 3. With correct X-API-Key -> 200 OK
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewBufferString(payload))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-API-Key", "secret-token-123")
	w3 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for correct X-API-Key, got %d", w3.Code)
	}

	// 4. With correct Bearer Authorization -> 200 OK
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewBufferString(payload))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", "Bearer secret-token-123")
	w4 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for correct Bearer auth, got %d", w4.Code)
	}
}
