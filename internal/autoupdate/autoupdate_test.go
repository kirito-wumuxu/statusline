package autoupdate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"statusline/internal/config"
)

func TestIsDailySyncDue(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, ".daily_sync")

	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.Local)

	// 1. Non-existent file -> due
	if !IsDailySyncDue(stateFile, now) {
		t.Fatalf("expected sync to be due when state file does not exist")
	}

	// 2. Marked for yesterday -> due
	yesterday := now.AddDate(0, 0, -1)
	if err := MarkDailySyncTriggered(stateFile, yesterday); err != nil {
		t.Fatalf("failed to mark state: %v", err)
	}
	if !IsDailySyncDue(stateFile, now) {
		t.Fatalf("expected sync to be due when last sync was yesterday")
	}

	// 3. Marked for today -> NOT due
	if err := MarkDailySyncTriggered(stateFile, now); err != nil {
		t.Fatalf("failed to mark state: %v", err)
	}
	if IsDailySyncDue(stateFile, now) {
		t.Fatalf("expected sync NOT to be due when already marked for today")
	}

	// 4. Corrupt state file -> due
	if err := os.WriteFile(stateFile, []byte("invalid-json-content"), 0644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	if !IsDailySyncDue(stateFile, now) {
		t.Fatalf("expected sync to be due when state file is corrupt")
	}
}

func TestMarkDailySyncTriggeredAndCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, ".daily_sync")
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.Local)

	// Triggered
	if err := MarkDailySyncTriggered(stateFile, now); err != nil {
		t.Fatalf("MarkDailySyncTriggered failed: %v", err)
	}
	data, _ := os.ReadFile(stateFile)
	if !bytes.Contains(data, []byte(`"last_status": "pending"`)) {
		t.Fatalf("expected pending status, got: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"last_date": "2026-11-16"`)) {
		t.Fatalf("expected 2026-11-16 date, got: %s", string(data))
	}

	// Completed success
	MarkDailySyncCompleted(stateFile, now, "v0.3.0", nil)
	data, _ = os.ReadFile(stateFile)
	if !bytes.Contains(data, []byte(`"last_status": "success"`)) {
		t.Fatalf("expected success status, got: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"upgraded_to": "v0.3.0"`)) {
		t.Fatalf("expected upgraded_to v0.3.0, got: %s", string(data))
	}

	// Completed error
	MarkDailySyncCompleted(stateFile, now, "", os.ErrPermission)
	data, _ = os.ReadFile(stateFile)
	if !bytes.Contains(data, []byte(`"last_status": "error"`)) {
		t.Fatalf("expected error status, got: %s", string(data))
	}
	if !bytes.Contains(data, []byte("permission denied")) {
		t.Fatalf("expected permission denied error message, got: %s", string(data))
	}
}

func TestTriggerDailySyncDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, ".daily_sync")
	os.Setenv("STATUSLINE_STATE_FILE", stateFile)
	defer os.Unsetenv("STATUSLINE_STATE_FILE")

	cfg := config.DefaultClientConfig()
	cfg.AutoSync.Enabled = false

	TriggerDailySyncIfDue(cfg, "v0.2.5")

	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("expected state file not to be created when auto_sync is disabled")
	}
}

func TestRunAutoUpdateSkipWhenAlreadyDoneToday(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, ".daily_sync")
	os.Setenv("STATUSLINE_STATE_FILE", stateFile)
	defer os.Unsetenv("STATUSLINE_STATE_FILE")

	now := time.Now()
	_ = MarkDailySyncTriggered(stateFile, now)

	var outBuf bytes.Buffer
	cfg := config.DefaultClientConfig()

	err := RunAutoUpdate(cfg, "v0.2.5", false, false, &outBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(outBuf.Bytes(), []byte("今日已完成")) {
		t.Fatalf("expected skip message, got: %s", outBuf.String())
	}
}

func TestRunAutoUpdateCheckMode(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, ".daily_sync")
	os.Setenv("STATUSLINE_STATE_FILE", stateFile)
	defer os.Unsetenv("STATUSLINE_STATE_FILE")

	var outBuf bytes.Buffer
	cfg := config.DefaultClientConfig()
	cfg.Report.Endpoint = "http://127.0.0.1:8628/api/v1/report"

	err := RunAutoUpdate(cfg, "v0.2.5", true, true, &outBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(outBuf.Bytes(), []byte("[check]")) {
		t.Fatalf("expected check mode indicator in output, got: %s", outBuf.String())
	}
}
