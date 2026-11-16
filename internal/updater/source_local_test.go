package updater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalSource(t *testing.T) {
	tmpDir := t.TempDir()

	platform := runtime.GOOS + "_" + runtime.GOARCH
	pkgName := fmt.Sprintf("statusline_v0.3.0_%s.tar.gz", platform)
	pkgPath := filepath.Join(tmpDir, pkgName)
	fakeContent := []byte("dummy binary package content")
	if err := os.WriteFile(pkgPath, fakeContent, 0644); err != nil {
		t.Fatalf("failed to write fake package: %v", err)
	}

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	checksumContent := fmt.Sprintf("abcdef1234567890  %s\n", pkgName)
	if err := os.WriteFile(checksumPath, []byte(checksumContent), 0644); err != nil {
		t.Fatalf("failed to write checksums: %v", err)
	}

	src := NewLocalSource(tmpDir)
	if src.Name() != "local" {
		t.Errorf("expected source name 'local', got %s", src.Name())
	}

	ctx := context.Background()
	rel, err := src.CheckRelease(ctx, "latest")
	if err != nil {
		t.Fatalf("CheckRelease failed: %v", err)
	}

	if rel.Version != "v0.3.0" {
		t.Errorf("expected version v0.3.0, got %s", rel.Version)
	}
	if rel.Checksums[pkgName] != "abcdef1234567890" {
		t.Errorf("expected checksum abcdef1234567890, got %s", rel.Checksums[pkgName])
	}

	assetPath, ok := rel.Assets[platform]
	if !ok || assetPath != pkgPath {
		t.Errorf("expected asset path %s, got %s", pkgPath, assetPath)
	}

	// Test download (copy)
	destPath := filepath.Join(tmpDir, "downloaded_pkg.tar.gz")
	var downloadedBytes int64
	err = src.Download(ctx, assetPath, destPath, func(d, total int64) {
		downloadedBytes = d
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if downloadedBytes != int64(len(fakeContent)) {
		t.Errorf("expected %d downloaded bytes, got %d", len(fakeContent), downloadedBytes)
	}

	destContent, err := os.ReadFile(destPath)
	if err != nil || string(destContent) != string(fakeContent) {
		t.Errorf("content mismatch in downloaded file")
	}
}
