package main

import (
	"bytes"
	"strings"
	"testing"

	"statusline/internal/config"
	"statusline/internal/node"
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
