package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"statusline/internal/config"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/session"
	"statusline/internal/storage"
)

func TestEndToEndSessionTUIIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sessions_test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()

	localNodeID := "node_e2e_local"
	foreignNodeID := "node_e2e_foreign"

	localRepoDir := filepath.Join(tmpDir, "local-repo")
	_ = os.MkdirAll(localRepoDir, 0755)

	// Create fake transcript
	transFile := filepath.Join(localRepoDir, "transcript.jsonl")
	transLines := []string{
		`{"step_index":1,"type":"USER_INPUT","content":"请为系统增加一个 TUI 会话管理器"}`,
		`{"step_index":2,"type":"PLANNER_RESPONSE","content":"已为您设计并实现了完整的 statusline-session 独立 TUI 二进制。"}`,
	}
	_ = os.WriteFile(transFile, []byte(strings.Join(transLines, "\n")+"\n"), 0644)

	// Ingest local session
	localNode := node.NodeInfo{NodeID: localNodeID, Hostname: "local-host", OSType: "linux", OSArch: "amd64"}
	localJSON := fmt.Sprintf(`{
		"product": "antigravity",
		"session_id": "sess_e2e_local",
		"cwd": %q,
		"transcript_path": %q,
		"git": {"repo_name": "local-repo", "branch": "main"},
		"model": {"id": "gemini-3.8-flash"}
	}`, localRepoDir, transFile)
	localEnv, _, err := model.ParseAndNormalize([]byte(localJSON), localNode)
	if err != nil {
		t.Fatalf("ParseAndNormalize local failed: %v", err)
	}
	_ = store.Ingest(localEnv)

	// Ingest foreign session
	foreignNode := node.NodeInfo{NodeID: foreignNodeID, Hostname: "remote-host", OSType: "linux", OSArch: "amd64"}
	foreignJSON := `{
		"product": "claude_code",
		"session_id": "sess_e2e_foreign",
		"cwd": "/other/machine/path",
		"git": {"repo_name": "remote-repo", "branch": "feat/remote"},
		"model": {"id": "claude-3-7-sonnet"}
	}`
	foreignEnv, _, err := model.ParseAndNormalize([]byte(foreignJSON), foreignNode)
	if err != nil {
		t.Fatalf("ParseAndNormalize foreign failed: %v", err)
	}
	_ = store.Ingest(foreignEnv)

	// Create SessionService configured for local node
	cfg := config.DefaultClientConfig()
	cfg.Node.ID = localNodeID

	svc := session.NewSessionService(store, cfg)
	if svc.GetLocalNodeID() != localNodeID {
		t.Errorf("expected local node ID %s, got %s", localNodeID, svc.GetLocalNodeID())
	}

	// 1. Verify default local filtering (only local node session appears)
	localItems, err := svc.ListSessions(context.Background(), true)
	if err != nil {
		t.Fatalf("ListSessions(true) failed: %v", err)
	}
	if len(localItems) != 1 {
		t.Fatalf("expected exactly 1 local session, got %d", len(localItems))
	}
	if localItems[0].Record.SessionID != "sess_e2e_local" {
		t.Errorf("expected sess_e2e_local, got %s", localItems[0].Record.SessionID)
	}
	if !localItems[0].HasValidCWD {
		t.Errorf("expected local repo dir to be recognized as valid CWD")
	}

	// 2. Verify all-nodes toggle includes both sessions
	allItems, err := svc.ListSessions(context.Background(), false)
	if err != nil {
		t.Fatalf("ListSessions(false) failed: %v", err)
	}
	if len(allItems) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(allItems))
	}

	// 3. Verify transcript preview extraction
	prompt, reply, err := session.LoadTranscriptPreview(localItems[0].Record.TranscriptPath)
	if err != nil {
		t.Fatalf("LoadTranscriptPreview failed: %v", err)
	}
	if !strings.Contains(prompt, "增加一个 TUI 会话管理器") {
		t.Errorf("expected prompt in preview, got: %s", prompt)
	}
	if !strings.Contains(reply, "statusline-session") {
		t.Errorf("expected reply in preview, got: %s", reply)
	}

	// 4. Verify resume command construction
	cmdName, _, cwd, shellCmd := session.BuildResumeCommand(localItems[0])
	if cmdName != "agy" {
		t.Errorf("expected cmdName agy, got %s", cmdName)
	}
	if cwd != localRepoDir {
		t.Errorf("expected cwd %s, got %s", localRepoDir, cwd)
	}
	if !strings.Contains(shellCmd, "cd '"+localRepoDir+"'") {
		t.Errorf("expected shell command to change directory, got: %s", shellCmd)
	}
}
