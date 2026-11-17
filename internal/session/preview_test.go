package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTranscriptPreview_Antigravity(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptFile := filepath.Join(tmpDir, "transcript.jsonl")

	lines := []string{
		`{"step_index":1,"type":"USER_INPUT","content":"请帮我重构一下数据库查询"}`,
		`{"step_index":2,"type":"PLANNER_RESPONSE","content":"好的，我已经通过优化索引将查询耗时降到 1ms。"}`,
	}
	_ = os.WriteFile(transcriptFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	prompt, reply, err := LoadTranscriptPreview(transcriptFile)
	if err != nil {
		t.Fatalf("LoadTranscriptPreview failed: %v", err)
	}

	if !strings.Contains(prompt, "请帮我重构一下数据库查询") {
		t.Errorf("expected prompt in preview, got: %q", prompt)
	}
	if !strings.Contains(reply, "优化索引") {
		t.Errorf("expected reply in preview, got: %q", reply)
	}
}

func TestLoadTranscriptPreview_ClaudeCode(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptFile := filepath.Join(tmpDir, "claude.jsonl")

	lines := []string{
		`{"type":"user","message":{"content":"Hello Claude, please explain Bubble Tea"}}`,
		`{"type":"assistant","message":{"content":"Bubble Tea is an Elm-architecture TUI framework for Go."}}`,
	}
	_ = os.WriteFile(transcriptFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	prompt, reply, err := LoadTranscriptPreview(transcriptFile)
	if err != nil {
		t.Fatalf("LoadTranscriptPreview failed: %v", err)
	}

	if !strings.Contains(prompt, "explain Bubble Tea") {
		t.Errorf("expected claude prompt in preview, got: %q", prompt)
	}
	if !strings.Contains(reply, "Elm-architecture") {
		t.Errorf("expected claude reply in preview, got: %q", reply)
	}
}

func TestLoadTranscriptPreview_NonExistent(t *testing.T) {
	prompt, reply, err := LoadTranscriptPreview("/non/existent/path.jsonl")
	if err == nil {
		t.Errorf("expected error for non-existent file, got prompt=%q, reply=%q", prompt, reply)
	}
}
