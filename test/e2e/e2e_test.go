package e2e

import (
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"statusline/internal/config"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/reporter"
	"statusline/internal/server"
	"statusline/internal/storage"
)

func executeMock(r io.Reader, w io.Writer, cfg *config.Config, n node.NodeInfo) {
	raw, _ := io.ReadAll(r)
	env, data, _ := model.ParseAndNormalize(raw, n)
	homeDir, _ := os.UserHomeDir()
	line := render.AssembleLine(data, homeDir)
	_, _ = io.WriteString(w, line)

	if cfg != nil && cfg.Server.Endpoint != "" {
		timeout := time.Duration(cfg.Server.TimeoutMS) * time.Millisecond
		reporter.ReportAsync(env, cfg.Server.Endpoint, timeout)
	}
}

func TestEndToEndPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "e2e.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	cfg := config.DefaultConfig()
	srv := server.NewServer(cfg, store, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Update client config to report to test server
	cfg.Server.Endpoint = ts.URL + "/api/v1/report"
	cfg.Server.TimeoutMS = 1000 // Give test ample timeout
	n := node.GetNodeInfo("node_e2e_01", "1.0.0")

	// Sample 1: Antigravity Payload
	agyJSON := `{
		"product": "antigravity",
		"session_id": "sess_agy_e2e",
		"email": "dev@example.com",
		"plan_tier": "Google AI Pro",
		"cwd": "/workspace/statusline",
		"model": {
			"id": "gemini-3.8-flash",
			"display_name": "Gemini 3.8 Flash (High)",
			"effort": "high"
		},
		"context_window": {
			"total_input_tokens": 15000,
			"total_output_tokens": 2500,
			"context_window_size": 1048576,
			"used_percentage": 1.67
		},
		"quota": {
			"gemini-weekly": {
				"remaining_fraction": 0.88,
				"reset_in_seconds": 7200,
				"reset_time": "2026-11-12T00:00:00Z"
			}
		}
	}`

	// Sample 2: Claude Code Payload (with GBK-style mojibake to test UTF-8 cleaner)
	claudeJSON := `{
		"session_id": "sess_claude_e2e",
		"prompt_id": "prompt_001",
		"session_name": "agentops 涓婃姤",
		"cwd": "/workspace/repo",
		"model": {
			"id": "claude-3-7-sonnet",
			"display_name": "Claude 3.7 Sonnet"
		},
		"effort": {
			"level": "high"
		},
		"cost": {
			"total_cost_usd": 0.0452,
			"total_duration_ms": 3200,
			"total_lines_added": 25
		},
		"worktree": {
			"branch": "main",
			"name": "default"
		},
		"context_window": {
			"total_input_tokens": 8200,
			"total_output_tokens": 1100,
			"context_window_size": 200000
		}
	}`

	// 1. Execute AGY statusline
	var outAGY bytes.Buffer
	executeMock(bytes.NewBufferString(agyJSON), &outAGY, cfg, n)

	plainAGY := render.StripANSI(outAGY.String())
	if !bytes.Contains([]byte(plainAGY), []byte("Gemini 3.8 Flash (High)")) {
		t.Errorf("expected AGY model in statusline, got %s", plainAGY)
	}

	// 2. Execute Claude Code statusline
	var outClaude bytes.Buffer
	executeMock(bytes.NewBufferString(claudeJSON), &outClaude, cfg, n)

	plainClaude := render.StripANSI(outClaude.String())
	t.Logf("plainClaude: %s", plainClaude)
	if !bytes.Contains([]byte(plainClaude), []byte("Claude 3.7 Sonnet")) {
		t.Errorf("expected Claude model in statusline, got %s", plainClaude)
	}

	// Wait for async reports to complete
	time.Sleep(300 * time.Millisecond)

	// Verify database content
	sessions, err := store.GetSessions("", 10)
	if err != nil {
		t.Fatalf("failed to query sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions stored in SQLite, got %d", len(sessions))
	}

	nodes, err := store.GetNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected 1 node stored in SQLite, got %d", len(nodes))
	}
	if nodes[0].NodeID != "node_e2e_01" {
		t.Errorf("expected node_e2e_01, got %s", nodes[0].NodeID)
	}

	quotas, err := store.GetAccountQuotas()
	if err != nil || len(quotas) != 1 {
		t.Fatalf("expected 1 quota record stored in SQLite, got %d", len(quotas))
	}
	if quotas[0].AccountEmail != "dev@example.com" {
		t.Errorf("expected dev@example.com quota, got %s", quotas[0].AccountEmail)
	}
}
