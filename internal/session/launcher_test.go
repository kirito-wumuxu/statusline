package session

import (
	"strings"
	"testing"

	"statusline/internal/storage"
)

func TestBuildResumeCommand(t *testing.T) {
	// 1. Claude Code with session ID
	itemClaude := SessionItem{
		Record: storage.SessionRecord{
			ProductType: "claude_code",
			SessionID:   "sess_claude_123",
			CWD:         "/home/user/my-project",
		},
	}
	cmdName, cmdArgs, cwd, fullShellCmd := BuildResumeCommand(itemClaude)
	if cmdName != "claude" {
		t.Errorf("expected cmdName 'claude', got %q", cmdName)
	}
	if len(cmdArgs) != 2 || cmdArgs[0] != "--resume" || cmdArgs[1] != "sess_claude_123" {
		t.Errorf("expected --resume sess_claude_123, got %v", cmdArgs)
	}
	if cwd != "/home/user/my-project" {
		t.Errorf("expected cwd, got %q", cwd)
	}
	if !strings.Contains(fullShellCmd, "cd '/home/user/my-project'") || !strings.Contains(fullShellCmd, "claude --resume sess_claude_123") {
		t.Errorf("expected full shell command, got %q", fullShellCmd)
	}

	// 2. Antigravity with spaces in cwd
	itemAgy := SessionItem{
		Record: storage.SessionRecord{
			ProductType: "antigravity",
			SessionID:   "sess_agy_abc",
			CWD:         "/home/user/my projects/web app",
		},
	}
	cmdName2, cmdArgs2, cwd2, fullShellCmd2 := BuildResumeCommand(itemAgy)
	if cmdName2 != "agy" {
		t.Errorf("expected cmdName 'agy', got %q", cmdName2)
	}
	if len(cmdArgs2) != 0 {
		t.Errorf("expected empty args for agy, got %v", cmdArgs2)
	}
	if cwd2 != "/home/user/my projects/web app" {
		t.Errorf("expected cwd, got %q", cwd2)
	}
	if !strings.Contains(fullShellCmd2, "cd '/home/user/my projects/web app'") || !strings.Contains(fullShellCmd2, "agy") {
		t.Errorf("expected full shell command, got %q", fullShellCmd2)
	}
}

func TestCheckWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	if !CheckWorkingDir(tmpDir) {
		t.Errorf("expected existing temp dir to be valid")
	}
	if CheckWorkingDir("/non/existent/dir/123456") {
		t.Errorf("expected non-existent dir to be invalid")
	}
	if CheckWorkingDir("") {
		t.Errorf("expected empty dir to be invalid")
	}
}

func TestSanitizeTmuxName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"my-project", "my-project"},
		{"repo:feature.v1", "repo-feature-v1"},
		{"project / test", "project---test"},
		{"...", "session"},
		{"", "session"},
	}

	for _, c := range cases {
		got := SanitizeTmuxName(c.input)
		if got != c.expected {
			t.Errorf("SanitizeTmuxName(%q): expected %q, got %q", c.input, c.expected, got)
		}
	}
}
