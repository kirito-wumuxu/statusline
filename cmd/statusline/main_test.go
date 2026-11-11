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
	cfg.Server.Endpoint = ts.URL
	cfg.Server.TimeoutMS = 500
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
	cfg.Server.Endpoint = "off"
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
	cfg.Server.Endpoint = "off"
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
