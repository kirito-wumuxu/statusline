package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeRecentUsage(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	sessionID := "test-session-123"

	// 1. 写入初始行 (普通用户请求，无 usage)
	line1 := `{"step_index": 0, "type": "USER_INPUT", "created_at": "2026-11-16T10:00:00Z"}` + "\n"
	if err := os.WriteFile(logPath, []byte(line1), 0644); err != nil {
		t.Fatal(err)
	}

	u1 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u1.InputTokens != 0 || u1.OutputTokens != 0 {
		t.Fatalf("expected 0 tokens, got %+v", u1)
	}

	// 2. 追加 Antigravity 格式的 PLANNER_RESPONSE
	line2 := `{"step_index": 1, "type": "PLANNER_RESPONSE", "input_tokens": 2570, "output_tokens": 89, "cache_read_tokens": 249741}` + "\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(line2)
	f.Close()

	u2 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u2.InputTokens != 2570 || u2.OutputTokens != 89 || u2.CacheReadTokens != 249741 {
		t.Fatalf("unexpected tokens for step 1: %+v", u2)
	}

	// 3. 再次探测，无新内容应返回 0
	u3 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u3.InputTokens != 0 || u3.OutputTokens != 0 || u3.CacheReadTokens != 0 {
		t.Fatalf("expected 0 tokens on second read, got %+v", u3)
	}

	// 4. 追加 Claude Code 格式日志
	line3 := `{"sessionId": "test-session-123", "message": {"model": "claude-3-7-sonnet", "usage": {"input_tokens": 1200, "output_tokens": 350, "cache_read_input_tokens": 8500, "cache_creation_input_tokens": 500}}}` + "\n"
	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(line3)
	f.Close()

	u4 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u4.InputTokens != 1200 || u4.OutputTokens != 350 || u4.CacheReadTokens != 8500 || u4.CacheCreationTokens != 500 {
		t.Fatalf("unexpected tokens for claude step: %+v", u4)
	}
}

func TestProbeRecentUsageTruncationReset(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	sessionID := "test-session-trunc"

	// 写入长日志
	line1 := `{"step_index": 1, "type": "PLANNER_RESPONSE", "input_tokens": 100, "output_tokens": 20}` + "\n"
	line2 := `{"step_index": 2, "type": "PLANNER_RESPONSE", "input_tokens": 200, "output_tokens": 40}` + "\n"
	if err := os.WriteFile(logPath, []byte(line1+line2), 0644); err != nil {
		t.Fatal(err)
	}

	u := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u.InputTokens != 300 || u.OutputTokens != 60 {
		t.Fatalf("expected 300 in, 60 out, got %+v", u)
	}

	// 文件被截断缩小
	newLine := `{"step_index": 1, "type": "PLANNER_RESPONSE", "input_tokens": 50, "output_tokens": 10}` + "\n"
	if err := os.WriteFile(logPath, []byte(newLine), 0644); err != nil {
		t.Fatal(err)
	}

	// 游标应自动检测并重置为 0，重新读取到 newLine
	uReset := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if uReset.InputTokens != 50 || uReset.OutputTokens != 10 {
		t.Fatalf("expected 50 in, 10 out after truncation reset, got %+v", uReset)
	}
}
