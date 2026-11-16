package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"statusline/internal/config"
	"statusline/internal/installer"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/vcs"
)

func TestExecuteStatuslineEmpty(t *testing.T) {
	// Review Focus #1: Empty stdin outputs fallback statusline and doesn't crash
	cfg := config.DefaultConfig()
	n := node.GetNodeInfo("node_test", "1.0.0")

	var in bytes.Buffer
	var out bytes.Buffer

	execute(&in, &out, cfg, n)

	result := out.String()
	if !strings.Contains(result, "[Gemini]") && !strings.Contains(result, "[~]") {
		t.Errorf("expected fallback statusline, got %q", result)
	}
}

func TestExecuteStatuslineValid(t *testing.T) {
	cfg := config.DefaultConfig()
	n := node.GetNodeInfo("node_test", "1.0.0")

	rawJSON := `{"product":"antigravity","model":{"id":"gemini-3.8-flash","display_name":"Flash"},"cwd":"/tmp"}`
	in := bytes.NewBufferString(rawJSON)
	var out bytes.Buffer

	execute(in, &out, cfg, n)

	result := out.String()
	if !strings.Contains(result, "[Flash]") {
		t.Errorf("expected [Flash] in output, got %q", result)
	}
}

func TestExecuteSynchronousReporting(t *testing.T) {
	// CRIT-1: Verify execute synchronously finishes reporting before returning so CLI exit doesn't kill it
	reported := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reported = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.Report.Endpoint = ts.URL
	cfg.Report.TimeoutMS = 500
	n := node.GetNodeInfo("node_test", "1.0.0")

	rawJSON := `{"product":"antigravity","model":{"id":"gemini-3.8-flash"}}`
	var out bytes.Buffer

	execute(bytes.NewBufferString(rawJSON), &out, cfg, n)

	// Since execute has returned, reporting must ALREADY be finished (no time.Sleep needed!)
	if !reported {
		t.Fatalf("expected execute to synchronously finish report before returning")
	}
}

func TestExecuteGitStatusIntegration(t *testing.T) {
	// IMP-1: Verify execute probes git status when cwd is present
	cfg := config.DefaultConfig()
	cfg.Report.Endpoint = "off"
	n := node.GetNodeInfo("node_test", "1.0.0")

	rawJSON := `{"product":"antigravity","model":{"id":"gemini-3.8-flash"},"cwd":"."}`
	var out bytes.Buffer

	execute(bytes.NewBufferString(rawJSON), &out, cfg, n)

	result := out.String()
	branch, _, _, _ := vcs.GetGitStatus(".", 500*time.Millisecond)
	if branch != "" && !strings.Contains(result, "("+branch) {
		t.Errorf("expected branch (%s) in statusline output, got %s", branch, result)
	}
}

func TestExecuteAccountAndSegments(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Report.Endpoint = "off"
	cfg.Display.Segments = []string{"account", "model"}
	n := node.GetNodeInfo("node_test", "1.0.0")

	rawJSON := `{"product":"antigravity","email":"kirito_wu@163.com","model":{"display_name":"Flash"},"cwd":"/tmp"}`
	var out bytes.Buffer

	execute(bytes.NewBufferString(rawJSON), &out, cfg, n)

	result := out.String()
	plain := render.StripANSI(result)
	if !strings.Contains(plain, "<kirito_wu@163.com>") {
		t.Errorf("expected account in output, got %q", plain)
	}
	if !strings.Contains(plain, "[Flash]") {
		t.Errorf("expected model in output, got %q", plain)
	}
	if strings.Contains(plain, "[/tmp]") {
		t.Errorf("expected dir omitted due to custom segments, got %q", plain)
	}
}

func TestInitCommandIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	binPath := "/usr/local/bin/statusline"

	var out bytes.Buffer
	err := installer.RunInit([]string{"-settings", settingsPath, "-bin", binPath}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	if !strings.Contains(string(data), binPath) {
		t.Errorf("expected binPath in settings JSON, got %s", string(data))
	}
}

func TestRunSyncCommand(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Mock Antigravity transcript
	agyDir := filepath.Join(tmpDir, ".gemini", "antigravity-cli", "brain", "test-sess", ".system_generated", "logs")
	if err := os.MkdirAll(agyDir, 0755); err != nil {
		t.Fatal(err)
	}
	transcriptData := `{"step_index": 1, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:00:00Z", "input_tokens": 1000, "output_tokens": 50}` + "\n"
	if err := os.WriteFile(filepath.Join(agyDir, "transcript.jsonl"), []byte(transcriptData), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Mock sync server
	var receivedBatch bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/sync/events" && r.Method == http.MethodPost {
			receivedBatch = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","received":1,"inserted":1,"skipped":0}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// 3. Write temp client.yaml
	cfgPath := filepath.Join(tmpDir, "client.yaml")
	cfgContent := "report:\n  endpoint: \"" + ts.URL + "/api/v1/report\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("STATUSLINE_CONFIG", cfgPath)

	var out bytes.Buffer
	if err := runSyncCommand([]string{"-home", tmpDir}, &out); err != nil {
		t.Fatalf("runSyncCommand failed: %v", err)
	}

	if !receivedBatch {
		t.Errorf("expected mock server to receive batch, but it didn't")
	}
	outStr := out.String()
	if !strings.Contains(outStr, "[3/3] 转录同步完成!") {
		t.Errorf("expected completion message, got: %s", outStr)
	}
	if !strings.Contains(outStr, "服务端新增入库: 1 条") {
		t.Errorf("expected 1 inserted, got: %s", outStr)
	}
}
