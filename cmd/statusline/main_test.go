package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"statusline/internal/config"
	"statusline/internal/node"
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
