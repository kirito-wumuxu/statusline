package model

import (
	"statusline/internal/node"
	"testing"
)

func TestParseAndNormalizeEmpty(t *testing.T) {
	n := node.GetNodeInfo("test-node", "1.0.0")
	env, data, err := ParseAndNormalize(nil, n)
	if err != nil {
		t.Fatalf("expected nil error on empty input, got %v", err)
	}
	if env == nil || data == nil {
		t.Fatalf("expected non-nil env and data on empty input")
	}
	if data.Model.DisplayName == "" {
		t.Fatalf("expected default model display name")
	}
}

func TestParseAndNormalizeAGY(t *testing.T) {
	raw := []byte(`{
		"product": "antigravity",
		"session_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
		"cwd": "/home/user/project",
		"model": {
			"id": "gemini-3.8-flash",
			"display_name": "Gemini 3.8 Flash (High)",
			"effort": "high"
		},
		"context_window": {
			"total_input_tokens": 12000,
			"total_output_tokens": 3000,
			"context_window_size": 1048576,
			"used_percentage": 1.43
		},
		"email": "dev@example.com",
		"plan_tier": "Google AI Pro",
		"quota": {
			"gemini-weekly": {
				"remaining_fraction": 0.85,
				"reset_in_seconds": 3600,
				"reset_time": "2026-11-12T00:00:00Z"
			}
		}
	}`)

	n := node.GetNodeInfo("test-node", "1.0.0")
	env, data, err := ParseAndNormalize(raw, n)
	if err != nil {
		t.Fatalf("ParseAndNormalize failed: %v", err)
	}

	if env.Normalized.ProductType != "antigravity" {
		t.Errorf("expected product_type antigravity, got %s", env.Normalized.ProductType)
	}
	if env.Normalized.ModelEffort != "high" {
		t.Errorf("expected model_effort high, got %s", env.Normalized.ModelEffort)
	}
	if env.Normalized.AccountEmail != "dev@example.com" {
		t.Errorf("expected email dev@example.com, got %s", env.Normalized.AccountEmail)
	}
	if env.Normalized.TotalInputTokens != 12000 {
		t.Errorf("expected 12000 input tokens, got %d", env.Normalized.TotalInputTokens)
	}
	if data.Model.DisplayName != "Gemini 3.8 Flash (High)" {
		t.Errorf("expected model display name, got %s", data.Model.DisplayName)
	}
}

func TestParseAndNormalizeClaudeCode(t *testing.T) {
	raw := []byte(`{
		"session_id": "c1a2d3e4-5f6a-7b8c-9d0e-1f2a3b4c5d6e",
		"prompt_id": "p1a2d3e4-5f6a-7b8c-9d0e-1f2a3b4c5d6e",
		"session_name": "test agentops",
		"cwd": "/workspace/app",
		"model": {
			"id": "claude-3-7-sonnet",
			"display_name": "Claude 3.7 Sonnet"
		},
		"effort": {
			"level": "high"
		},
		"cost": {
			"total_cost_usd": 0.0452,
			"total_duration_ms": 3500,
			"total_api_duration_ms": 2800,
			"total_lines_added": 12,
			"total_lines_removed": 4
		},
		"workspace": {
			"repo": {
				"host": "github.com",
				"owner": "anthropic",
				"name": "claude-code"
			}
		},
		"worktree": {
			"branch": "main",
			"name": "default"
		},
		"context_window": {
			"total_input_tokens": 8500,
			"total_output_tokens": 1200,
			"context_window_size": 200000
		}
	}`)

	n := node.GetNodeInfo("test-node", "1.0.0")
	env, data, err := ParseAndNormalize(raw, n)
	if err != nil {
		t.Fatalf("ParseAndNormalize failed: %v", err)
	}

	if env.Normalized.ProductType != "claude_code" {
		t.Errorf("expected product_type claude_code, got %s", env.Normalized.ProductType)
	}
	if env.Normalized.ModelEffort != "high" {
		t.Errorf("expected model_effort high from effort.level, got %s", env.Normalized.ModelEffort)
	}
	if env.Normalized.TotalCostUSD != 0.0452 {
		t.Errorf("expected cost 0.0452, got %f", env.Normalized.TotalCostUSD)
	}
	if env.Normalized.GitRepoOwner != "anthropic" || env.Normalized.GitRepoName != "claude-code" {
		t.Errorf("expected git repo anthropic/claude-code, got %s/%s", env.Normalized.GitRepoOwner, env.Normalized.GitRepoName)
	}
	if env.Normalized.GitBranch != "main" {
		t.Errorf("expected branch main, got %s", env.Normalized.GitBranch)
	}
	if data.Model.ID != "claude-3-7-sonnet" {
		t.Errorf("expected model id, got %s", data.Model.ID)
	}
}
