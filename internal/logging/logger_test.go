package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggingDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	origResolve := resolveExeDir
	resolveExeDir = func() string { return tmpDir }
	defer func() { resolveExeDir = origResolve }()

	cleanup := Setup(false, "info", "text", "test.log")
	defer cleanup()

	// Handler should be nopHandler and disabled for all levels
	if slog.Default().Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("expected logger to be disabled when enabled=false")
	}

	slog.Info("this should not be written", "key", "val")

	logFile := filepath.Join(tmpDir, "test.log")
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("expected log file not to exist, but found: %s", logFile)
	}
}

func TestLoggingEnabledText(t *testing.T) {
	tmpDir := t.TempDir()
	origResolve := resolveExeDir
	resolveExeDir = func() string { return tmpDir }
	defer func() { resolveExeDir = origResolve }()

	cleanup := Setup(true, "debug", "text", "test_text.log")
	slog.Debug("test debug text log", "module", "render", "latency", 12)
	slog.Info("test info text log", "status", "ok")
	cleanup()

	logFile := filepath.Join(tmpDir, "test_text.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read created log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "test debug text log") {
		t.Errorf("expected log to contain debug message, got: %s", content)
	}
	if !strings.Contains(content, "module=render") {
		t.Errorf("expected log to contain attr module=render, got: %s", content)
	}
	if !strings.Contains(content, "test info text log") {
		t.Errorf("expected log to contain info message, got: %s", content)
	}
}

func TestLoggingEnabledJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origResolve := resolveExeDir
	resolveExeDir = func() string { return tmpDir }
	defer func() { resolveExeDir = origResolve }()

	cleanup := Setup(true, "info", "json", "test_json.log")
	slog.Info("test json record", "session_id", "sess_123", "tokens", 450)
	cleanup()

	logFile := filepath.Join(tmpDir, "test_json.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read json log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected non-empty json log file")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("failed to parse json log record: %v\ncontent: %s", err, lines[0])
	}

	if parsed["msg"] != "test json record" {
		t.Errorf("expected msg 'test json record', got %v", parsed["msg"])
	}
	if parsed["session_id"] != "sess_123" {
		t.Errorf("expected session_id 'sess_123', got %v", parsed["session_id"])
	}
}

func TestLoggingLevelFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	origResolve := resolveExeDir
	resolveExeDir = func() string { return tmpDir }
	defer func() { resolveExeDir = origResolve }()

	// Warn level should ignore Debug and Info
	cleanup := Setup(true, "warn", "text", "test_warn.log")
	slog.Debug("hidden debug")
	slog.Info("hidden info")
	slog.Warn("visible warn")
	slog.Error("visible error")
	cleanup()

	logFile := filepath.Join(tmpDir, "test_warn.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "hidden debug") {
		t.Errorf("expected debug to be filtered out")
	}
	if strings.Contains(content, "hidden info") {
		t.Errorf("expected info to be filtered out")
	}
	if !strings.Contains(content, "visible warn") {
		t.Errorf("expected warn to be logged")
	}
	if !strings.Contains(content, "visible error") {
		t.Errorf("expected error to be logged")
	}
}
