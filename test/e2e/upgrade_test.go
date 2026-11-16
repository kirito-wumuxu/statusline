package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"statusline/internal/config"
	"statusline/internal/server"
	"statusline/internal/storage"
	"statusline/internal/updater"
)

func TestEndToEndUpgradePipeline(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Setup server releases directory
	releasesDir := filepath.Join(tmpDir, "server_releases")
	if err := os.MkdirAll(releasesDir, 0755); err != nil {
		t.Fatalf("failed to create releases dir: %v", err)
	}

	platform := runtime.GOOS + "_" + runtime.GOARCH
	var archiveName string
	var archiveBuf bytes.Buffer

	// Build a valid archive with a new fake statusline binary
	newBinaryContent := []byte("#!/bin/sh\necho 'v0.2.0'\n")

	if runtime.GOOS == "windows" {
		archiveName = fmt.Sprintf("statusline_v0.2.0_%s.zip", platform)
		// Zip creation for test
		zBuf := new(bytes.Buffer)
		zw := gzip.NewWriter(zBuf) // fallback dummy if zip not used, but let's do proper tar.gz on non-windows
		zw.Close()
	} else {
		archiveName = fmt.Sprintf("statusline_v0.2.0_%s.tar.gz", platform)
		gw := gzip.NewWriter(&archiveBuf)
		tw := tar.NewWriter(gw)

		header := &tar.Header{
			Name: "statusline_v0.2.0_" + platform + "/statusline",
			Mode: 0755,
			Size: int64(len(newBinaryContent)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader failed: %v", err)
		}
		if _, err := tw.Write(newBinaryContent); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		tw.Close()
		gw.Close()
	}

	archiveBytes := archiveBuf.Bytes()
	archivePath := filepath.Join(releasesDir, archiveName)
	if err := os.WriteFile(archivePath, archiveBytes, 0644); err != nil {
		t.Fatalf("failed to write archive file: %v", err)
	}

	// Generate checksums.txt
	h := sha256.New()
	h.Write(archiveBytes)
	checksumHex := hex.EncodeToString(h.Sum(nil))
	checksumContent := fmt.Sprintf("%s  %s\n", checksumHex, archiveName)
	if err := os.WriteFile(filepath.Join(releasesDir, "checksums.txt"), []byte(checksumContent), 0644); err != nil {
		t.Fatalf("failed to write checksums.txt: %v", err)
	}

	// 2. Start server
	dbPath := filepath.Join(tmpDir, "server.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	serverCfg := config.DefaultServerConfig()
	serverCfg.Storage.ReleasesDir = releasesDir
	srv := server.NewServer(serverCfg, store, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// 3. Setup client environment with fake running binary
	clientBinDir := filepath.Join(tmpDir, "client_bin")
	if err := os.MkdirAll(clientBinDir, 0755); err != nil {
		t.Fatalf("failed to create client bin dir: %v", err)
	}
	targetExecutable := filepath.Join(clientBinDir, "statusline")
	oldBinaryContent := []byte("#!/bin/sh\necho 'v0.1.0'\n")
	if err := os.WriteFile(targetExecutable, oldBinaryContent, 0755); err != nil {
		t.Fatalf("failed to write old binary: %v", err)
	}

	clientCfg := config.DefaultClientConfig()
	clientCfg.Report.Endpoint = ts.URL + "/api/v1/report"

	// 4. Execute upgrade via updater
	var outBuf bytes.Buffer
	opts := updater.UpgradeOptions{
		TargetTag:        "v0.2.0",
		ForcedSource:     "server",
		Yes:              true,
		CurrentVer:       "v0.1.0",
		TargetExecutable: targetExecutable,
		ClientConfig:     clientCfg,
	}

	err = updater.ExecuteUpgrade(opts, &outBuf)
	if err != nil {
		t.Fatalf("ExecuteUpgrade failed: %v\nOutput: %s", err, outBuf.String())
	}

	// 5. Verify the target executable was atomically replaced with newBinaryContent
	updatedContent, err := os.ReadFile(targetExecutable)
	if err != nil {
		t.Fatalf("failed to read updated executable: %v", err)
	}
	if !bytes.Equal(updatedContent, newBinaryContent) {
		t.Errorf("executable content mismatch, expected %q, got %q", string(newBinaryContent), string(updatedContent))
	}

	t.Logf("Upgrade output:\n%s", outBuf.String())

	// 6. Test upgrade when already on latest version
	var outLatest bytes.Buffer
	optsLatest := updater.UpgradeOptions{
		TargetTag:        "latest",
		ForcedSource:     "server",
		Yes:              true,
		CurrentVer:       "v0.2.0",
		TargetExecutable: targetExecutable,
		ClientConfig:     clientCfg,
	}
	if err := updater.ExecuteUpgrade(optsLatest, &outLatest); err != nil {
		t.Fatalf("expected clean exit when already up-to-date, got %v", err)
	}
	if !bytes.Contains(outLatest.Bytes(), []byte("当前已是最新版本")) {
		t.Errorf("expected up-to-date notice, got %s", outLatest.String())
	}

	// 7. Test -check flag (does not overwrite binary)
	var outCheck bytes.Buffer
	optsCheck := updater.UpgradeOptions{
		CheckOnly:        true,
		TargetTag:        "latest",
		ForcedSource:     "server",
		CurrentVer:       "v0.1.0",
		TargetExecutable: targetExecutable,
		ClientConfig:     clientCfg,
	}
	if err := updater.ExecuteUpgrade(optsCheck, &outCheck); err != nil {
		t.Fatalf("expected clean exit with -check flag, got %v", err)
	}
	if !bytes.Contains(outCheck.Bytes(), []byte("-check 模式")) {
		t.Errorf("expected check mode notice, got %s", outCheck.String())
	}
}

func TestServerSelfUpgradePipeline(t *testing.T) {
	tmpDir := t.TempDir()

	platform := runtime.GOOS + "_" + runtime.GOARCH
	var archiveName string
	var archiveBuf bytes.Buffer

	newServerBinaryContent := []byte("#!/bin/sh\necho 'server v0.3.0'\n")
	newClientBinaryContent := []byte("#!/bin/sh\necho 'client v0.3.0'\n")

	archiveName = fmt.Sprintf("statusline_v0.3.0_%s.tar.gz", platform)
	gw := gzip.NewWriter(&archiveBuf)
	tw := tar.NewWriter(gw)

	// Add statusline-server into archive
	hServer := &tar.Header{
		Name: "statusline_v0.3.0_" + platform + "/statusline-server",
		Mode: 0755,
		Size: int64(len(newServerBinaryContent)),
	}
	_ = tw.WriteHeader(hServer)
	_, _ = tw.Write(newServerBinaryContent)

	// Add statusline into archive
	hClient := &tar.Header{
		Name: "statusline_v0.3.0_" + platform + "/statusline",
		Mode: 0755,
		Size: int64(len(newClientBinaryContent)),
	}
	_ = tw.WriteHeader(hClient)
	_, _ = tw.Write(newClientBinaryContent)

	tw.Close()
	gw.Close()

	archiveBytes := archiveBuf.Bytes()
	releasesDir := filepath.Join(tmpDir, "local_releases")
	_ = os.MkdirAll(releasesDir, 0755)
	_ = os.WriteFile(filepath.Join(releasesDir, archiveName), archiveBytes, 0644)

	// Setup fake server running binary
	serverBinDir := filepath.Join(tmpDir, "server_bin")
	_ = os.MkdirAll(serverBinDir, 0755)
	targetServerExecutable := filepath.Join(serverBinDir, "statusline-server")
	targetClientExecutable := filepath.Join(serverBinDir, "statusline")
	oldServerContent := []byte("#!/bin/sh\necho 'server v0.2.0'\n")
	oldClientContent := []byte("#!/bin/sh\necho 'client v0.2.0'\n")
	_ = os.WriteFile(targetServerExecutable, oldServerContent, 0755)
	_ = os.WriteFile(targetClientExecutable, oldClientContent, 0755)

	// Test ExecuteUpgrade on statusline-server with local source and -all
	var outBuf bytes.Buffer
	opts := updater.UpgradeOptions{
		TargetTag:        "v0.3.0",
		ForcedSource:     "local",
		UpgradeAll:       true,
		Yes:              true,
		CurrentVer:       "v0.2.0",
		TargetExecutable: targetServerExecutable,
	}

	// Point local source to releasesDir
	os.Setenv("STATUSLINE_RELEASES_DIR", releasesDir)
	defer os.Unsetenv("STATUSLINE_RELEASES_DIR")

	localSrc := updater.NewLocalSource(releasesDir)
	rel, err := localSrc.CheckRelease(context.Background(), "latest")
	if err != nil {
		t.Fatalf("LocalSource CheckRelease failed: %v", err)
	}
	if rel.Version != "v0.3.0" {
		t.Fatalf("expected version v0.3.0, got %s", rel.Version)
	}

	err = updater.ExecuteUpgrade(opts, &outBuf)
	if err != nil {
		t.Fatalf("Server ExecuteUpgrade failed: %v\nOutput: %s", err, outBuf.String())
	}

	// Verify statusline-server replaced
	upServer, _ := os.ReadFile(targetServerExecutable)
	if !bytes.Equal(upServer, newServerBinaryContent) {
		t.Errorf("statusline-server content mismatch, expected %q, got %q", string(newServerBinaryContent), string(upServer))
	}

	// Verify statusline client also replaced (because of -all)
	upClient, _ := os.ReadFile(targetClientExecutable)
	if !bytes.Equal(upClient, newClientBinaryContent) {
		t.Errorf("statusline client content mismatch, expected %q, got %q", string(newClientBinaryContent), string(upClient))
	}

	if !bytes.Contains(outBuf.Bytes(), []byte("statusline-server 已成功替换为 v0.3.0")) {
		t.Errorf("expected server replacement output, got: %s", outBuf.String())
	}
	if !bytes.Contains(outBuf.Bytes(), []byte("statusline 已同时成功替换为 v0.3.0")) {
		t.Errorf("expected client replacement output, got: %s", outBuf.String())
	}
}
