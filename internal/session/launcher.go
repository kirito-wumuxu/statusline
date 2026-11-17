package session

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// BuildResumeCommand analyzes the session item and generates the appropriate CLI resume command.
func BuildResumeCommand(item SessionItem) (cmdName string, cmdArgs []string, workingDir string, fullShellCmd string) {
	prod := strings.ToLower(strings.TrimSpace(item.Record.ProductType))
	workingDir = item.Record.CWD

	switch {
	case strings.Contains(prod, "claude"):
		cmdName = "claude"
		if item.Record.SessionID != "" {
			cmdArgs = []string{"--resume", item.Record.SessionID}
		}
	case strings.Contains(prod, "antigravity") || strings.Contains(prod, "agy"):
		cmdName = "agy"
	default:
		cmdName = prod
		if cmdName == "" {
			cmdName = "claude"
		}
	}

	// Build full shell command line
	cmdParts := append([]string{cmdName}, cmdArgs...)
	cmdJoined := strings.Join(cmdParts, " ")

	if workingDir != "" {
		escapedCWD := strings.ReplaceAll(workingDir, "'", "'\\''")
		fullShellCmd = fmt.Sprintf("cd '%s' && %s", escapedCWD, cmdJoined)
	} else {
		fullShellCmd = cmdJoined
	}

	return cmdName, cmdArgs, workingDir, fullShellCmd
}

// CheckWorkingDir verifies if the specified working directory exists on this machine.
func CheckWorkingDir(cwd string) bool {
	if cwd == "" {
		return false
	}
	fi, err := os.Stat(cwd)
	return err == nil && fi.IsDir()
}

// LaunchInTmux creates a new window in the active tmux session to run the command.
func LaunchInTmux(fullCmd, windowName, cwd string) error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("当前不在 tmux 环境中，无法在新窗口打开")
	}

	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("系统中未找到 tmux 可执行文件: %w", err)
	}

	args := []string{"new-window"}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	args = append(args, fullCmd)

	cmd := exec.Command(tmuxBin, args...)
	return cmd.Run()
}

// CopyToClipboard attempts to copy text using OSC 52 sequence and native platform fallbacks.
func CopyToClipboard(text string) error {
	// 1. Try OSC 52 ANSI sequence to stdout/stderr (works over SSH, inside modern terminals)
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	osc52 := fmt.Sprintf("\x1b]52;c;%s\x07", b64)
	_, _ = os.Stdout.WriteString(osc52)

	// 2. Native OS clipboard tool fallbacks
	switch runtime.GOOS {
	case "darwin":
		if bin, err := exec.LookPath("pbcopy"); err == nil {
			cmd := exec.Command(bin)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	case "linux":
		if bin, err := exec.LookPath("wl-copy"); err == nil { // Wayland
			cmd := exec.Command(bin)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
		if bin, err := exec.LookPath("xclip"); err == nil { // X11
			cmd := exec.Command(bin, "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
		if bin, err := exec.LookPath("xsel"); err == nil {
			cmd := exec.Command(bin, "--clipboard", "--input")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	case "windows":
		if bin, err := exec.LookPath("clip.exe"); err == nil {
			cmd := exec.Command(bin)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}

	return nil
}
