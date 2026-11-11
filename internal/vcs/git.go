package vcs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	ansiReset = "\033[0m"
	ansiWhite = "\033[37m"
	ansiGreen = "\033[32m"
	ansiRed   = "\033[31m"
)

// GetGitStatus extracts current branch name, dirty flag, and ahead/behind counts with a strict timeout.
func GetGitStatus(dir string, timeout time.Duration) (branch string, dirty bool, ahead int, behind int) {
	if dir == "" {
		return "", false, 0, 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 1. Get branch name
	cmdBranch := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmdBranch.Dir = dir
	if out, err := cmdBranch.Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	} else {
		return "", false, 0, 0
	}

	// 2. Check if dirty (unstaged/uncommitted files)
	cmdDirty := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmdDirty.Dir = dir
	if out, err := cmdDirty.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		dirty = true
	}

	// 3. Ahead / behind upstream
	cmdCounts := exec.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	cmdCounts.Dir = dir
	if out, err := cmdCounts.Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) >= 2 {
			ahead, _ = strconv.Atoi(fields[0])
			behind, _ = strconv.Atoi(fields[1])
		}
	}

	return branch, dirty, ahead, behind
}

// GetSyncStatus retrieves the git commits ahead and behind upstream within a strict timeout formatted with ANSI colors.
func GetSyncStatus(dir string, timeout time.Duration) string {
	defaultStatus := fmt.Sprintf("%s↑0 ↓0%s", ansiWhite, ansiReset)
	if dir == "" {
		return defaultStatus
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return defaultStatus
	}

	fields := strings.Fields(string(output))
	if len(fields) < 2 {
		return defaultStatus
	}

	ahead, err1 := strconv.Atoi(fields[0])
	behind, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return defaultStatus
	}

	aheadStr := fmt.Sprintf("%s↑0%s", ansiWhite, ansiReset)
	if ahead > 0 {
		aheadStr = fmt.Sprintf("%s↑%d%s", ansiGreen, ahead, ansiReset)
	}

	behindStr := fmt.Sprintf("%s↓0%s", ansiWhite, ansiReset)
	if behind > 0 {
		behindStr = fmt.Sprintf("%s↓%d%s", ansiRed, behind, ansiReset)
	}

	return fmt.Sprintf("%s %s", aheadStr, behindStr)
}
