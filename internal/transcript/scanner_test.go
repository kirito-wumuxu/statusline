package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAndScanTranscripts(t *testing.T) {
	tempHome := t.TempDir()

	// 1. Setup mock Antigravity transcript
	agyDir := filepath.Join(tempHome, ".gemini", "antigravity-cli", "brain", "conv-1234", ".system_generated", "logs")
	if err := os.MkdirAll(agyDir, 0755); err != nil {
		t.Fatal(err)
	}
	agyContent := `{"step_index": 1, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:00:00Z", "input_tokens": 1000, "output_tokens": 50, "cache_read_tokens": 5000}` + "\n" +
		`{"step_index": 2, "type": "USER_INPUT", "created_at": "2026-11-16T10:01:00Z"}` + "\n" +
		`{"step_index": 3, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:02:00Z", "input_tokens": 1500, "output_tokens": 100, "cache_read_tokens": 6000}` + "\n"
	if err := os.WriteFile(filepath.Join(agyDir, "transcript.jsonl"), []byte(agyContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Setup mock Claude Code transcript
	claudeDir := filepath.Join(tempHome, ".claude", "projects", "proj-abc")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	claudeContent := `{"timestamp": "2026-11-16T10:05:00Z", "message": {"id": "msg_01", "model": "claude-3-7-sonnet", "usage": {"input_tokens": 2000, "output_tokens": 300, "cache_read_input_tokens": 10000, "cache_creation_input_tokens": 500}}}` + "\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "session-xyz.jsonl"), []byte(claudeContent), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := DiscoverAndScanTranscripts(tempHome, "test-node")
	if err != nil {
		t.Fatalf("DiscoverAndScanTranscripts failed: %v", err)
	}

	if summary.FilesScanned != 2 {
		t.Errorf("expected 2 files scanned, got %d", summary.FilesScanned)
	}
	if summary.TotalEvents != 3 {
		t.Errorf("expected 3 total events (2 from AGY, 1 from Claude), got %d", summary.TotalEvents)
	}

	// Verify events
	var agyCount, claudeCount int
	for _, e := range summary.Events {
		if e.ProductType == "antigravity" {
			agyCount++
			if e.SessionID != "conv-1234" {
				t.Errorf("expected AGY session conv-1234, got %s", e.SessionID)
			}
		} else if e.ProductType == "claude_code" {
			claudeCount++
			if e.SessionID != "session-xyz" {
				t.Errorf("expected Claude session session-xyz, got %s", e.SessionID)
			}
			if e.PromptID != "msg_01" {
				t.Errorf("expected Claude PromptID msg_01, got %s", e.PromptID)
			}
			if e.CurrentInputTokens != 2000 || e.CurrentOutputTokens != 300 || e.CacheReadTokens != 10000 || e.CacheCreationTokens != 500 {
				t.Errorf("unexpected Claude token counts: %+v", e)
			}
		}
	}

	if agyCount != 2 || claudeCount != 1 {
		t.Errorf("expected 2 AGY and 1 Claude events, got %d and %d", agyCount, claudeCount)
	}
}
