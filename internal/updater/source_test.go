package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"statusline/internal/config"
)

func TestServerSourceSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"status":"ok","latest_version":"v0.1.6","upgrade_supported":true,"assets":{"linux_amd64":"/api/v1/upgrade/download?platform=linux_amd64"},"checksums":{"statusline_v0.1.6_linux_amd64.tar.gz":"abcdef123456"}}`)
			return
		}
		if r.URL.Path == "/api/v1/upgrade/download" {
			w.Header().Set("Content-Type", "application/octet-stream")
			fmt.Fprintln(w, "mock downloaded package content")
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	cfg := &config.Config{
		Report: config.ReportConfig{Endpoint: ts.URL + "/api/v1/report"},
	}

	src, err := ResolveSource(cfg, "auto")
	if err != nil {
		t.Fatalf("failed to resolve source: %v", err)
	}
	if src.Name() != "server" {
		t.Errorf("expected server source, got %s", src.Name())
	}

	rel, err := src.CheckRelease(context.Background(), "latest")
	if err != nil {
		t.Fatalf("CheckRelease failed: %v", err)
	}
	if rel.Version != "v0.1.6" {
		t.Errorf("expected v0.1.6, got %s", rel.Version)
	}
	if rel.Assets["linux_amd64"] == "" {
		t.Errorf("expected asset url for linux_amd64")
	}

	// Test Download
	tmpDir := t.TempDir()
	destFile := filepath.Join(tmpDir, "downloaded.pkg")
	var downloadedBytes int64
	err = src.Download(context.Background(), rel.Assets["linux_amd64"], destFile, func(d, total int64) {
		downloadedBytes = d
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if downloadedBytes <= 0 {
		t.Errorf("expected downloadedBytes > 0, got %d", downloadedBytes)
	}
	content, _ := os.ReadFile(destFile)
	if string(content) != "mock downloaded package content\n" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestResolveSourceFallbackToGitHubWhenServerUnavailable(t *testing.T) {
	t.Setenv("STATUSLINE_RELEASES_DIR", "off") // disable local dir scan to test github fallback

	cfg := &config.Config{
		Report: config.ReportConfig{Endpoint: "http://127.0.0.1:54321/api/v1/report"}, // offline server
	}

	src, err := ResolveSource(cfg, "auto")
	if err != nil {
		t.Fatalf("failed to resolve source: %v", err)
	}
	if src.Name() != "github" {
		t.Errorf("expected fallback to github, got %s", src.Name())
	}
}

func TestResolveSourceExplicitForcedSource(t *testing.T) {
	cfg := &config.Config{
		Report: config.ReportConfig{Endpoint: "http://127.0.0.1:8628/api/v1/report"},
	}

	src, err := ResolveSource(cfg, "github")
	if err != nil {
		t.Fatalf("failed to resolve source: %v", err)
	}
	if src.Name() != "github" {
		t.Errorf("expected forced github source, got %s", src.Name())
	}
}
