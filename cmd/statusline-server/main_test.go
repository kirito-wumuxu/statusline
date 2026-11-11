package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/server"
	"statusline/internal/storage"
)

func TestServerWebAssets(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "web_test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	srv := server.NewServer(cfg, store, webFS)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from web root, got %d", w.Code)
	}
}
