package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/storage"
)

func TestHandlerVersionAndDownload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "server_upgrade_test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	releasesDir := filepath.Join(tmpDir, "releases")
	_ = os.MkdirAll(releasesDir, 0755)
	dummyPkg := filepath.Join(releasesDir, "statusline_v0.1.6_linux_amd64.tar.gz")
	_ = os.WriteFile(dummyPkg, []byte("fake binary package"), 0644)
	csFile := filepath.Join(releasesDir, "checksums.txt")
	_ = os.WriteFile(csFile, []byte("abcdef1234567890 statusline_v0.1.6_linux_amd64.tar.gz\n"), 0644)

	cfg := &config.ServerConfig{
		Server:  config.ServerListenConfig{Host: "127.0.0.1", Port: 8628, APIKey: "test-secret"},
		Storage: config.StorageConfig{DBPath: dbPath, ReleasesDir: releasesDir},
	}

	srv := NewServer(cfg, store, nil)

	// Test GET /api/v1/version
	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Test GET /api/v1/upgrade/download
	reqDl := httptest.NewRequest("GET", "/api/v1/upgrade/download?platform=linux_amd64", nil)
	wDl := httptest.NewRecorder()
	srv.router.ServeHTTP(wDl, reqDl)

	if wDl.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d", wDl.Code)
	}
	if wDl.Body.String() != "fake binary package" {
		t.Errorf("download content mismatch, got %s", wDl.Body.String())
	}

	// Test POST /api/v1/upgrade/upload without API key -> 401
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("files", "statusline_v0.1.7_linux_amd64.tar.gz")
	_, _ = io.WriteString(part, "v0.1.7 content")
	_ = writer.Close()

	reqUpNoAuth := httptest.NewRequest("POST", "/api/v1/upgrade/upload", &body)
	reqUpNoAuth.Header.Set("Content-Type", writer.FormDataContentType())
	wUpNoAuth := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpNoAuth, reqUpNoAuth)
	if wUpNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without api key, got %d", wUpNoAuth.Code)
	}

	// Test POST /api/v1/upgrade/upload with API key -> 200
	var body2 bytes.Buffer
	writer2 := multipart.NewWriter(&body2)
	part2, _ := writer2.CreateFormFile("files", "statusline_v0.1.7_linux_amd64.tar.gz")
	_, _ = io.WriteString(part2, "v0.1.7 content")
	_ = writer2.Close()

	reqUpAuth := httptest.NewRequest("POST", "/api/v1/upgrade/upload", &body2)
	reqUpAuth.Header.Set("Content-Type", writer2.FormDataContentType())
	reqUpAuth.Header.Set("X-API-Key", "test-secret")
	wUpAuth := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpAuth, reqUpAuth)
	if wUpAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with api key, got %d: %s", wUpAuth.Code, wUpAuth.Body.String())
	}

	// Verify file was written to releasesDir
	uploadedPath := filepath.Join(releasesDir, "statusline_v0.1.7_linux_amd64.tar.gz")
	if _, err := os.Stat(uploadedPath); err != nil {
		t.Fatalf("expected uploaded file to exist: %v", err)
	}
}
