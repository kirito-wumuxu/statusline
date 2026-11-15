package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "test.tar.gz")
	targetContent := "#!/bin/sh\necho statusline"

	// Create test .tar.gz
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{Name: "statusline_v0.1.6/statusline", Mode: 0755, Size: int64(len(targetContent))}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(targetContent))
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	destBinary := filepath.Join(tmpDir, "extracted_statusline")
	if err := ExtractBinaryFromArchive(tarPath, "statusline", destBinary); err != nil {
		t.Fatalf("failed to extract: %v", err)
	}

	content, _ := os.ReadFile(destBinary)
	if string(content) != targetContent {
		t.Errorf("extracted content mismatch, got %s", string(content))
	}
}

func TestExtractFromZip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	targetContent := "fake statusline.exe content"

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("statusline_v0.1.6/statusline.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(targetContent))
	_ = zw.Close()
	_ = f.Close()

	destBinary := filepath.Join(tmpDir, "extracted_statusline.exe")
	if err := ExtractBinaryFromArchive(zipPath, "statusline.exe", destBinary); err != nil {
		t.Fatalf("failed to extract from zip: %v", err)
	}

	content, _ := os.ReadFile(destBinary)
	if string(content) != targetContent {
		t.Errorf("extracted zip content mismatch, got %s", string(content))
	}
}

func TestVerifySHA256(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hash.txt")
	_ = os.WriteFile(testFile, []byte("statusline test hash"), 0644)

	h := sha256.Sum256([]byte("statusline test hash"))
	expectedHex := hex.EncodeToString(h[:])

	if err := VerifySHA256(testFile, expectedHex); err != nil {
		t.Fatalf("expected verify pass, got %v", err)
	}
	if err := VerifySHA256(testFile, "0000000000000000"); err == nil {
		t.Fatalf("expected verify failure on mismatch hash")
	}
}
