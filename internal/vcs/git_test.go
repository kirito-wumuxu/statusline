package vcs

import (
	"testing"
	"time"
)

func TestGetGitStatus(t *testing.T) {
	branch, dirty, ahead, behind := GetGitStatus(".", 500*time.Millisecond)
	if branch == "" {
		t.Fatalf("expected non-empty branch in git repository, got empty")
	}
	t.Logf("git status: branch=%s, dirty=%v, ahead=%d, behind=%d", branch, dirty, ahead, behind)
}
