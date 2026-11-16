//go:build !windows

package autoupdate

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnDetachedAutoUpdate spawns statusline in the background detached from the current process session.
func SpawnDetachedAutoUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "auto-update")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	return cmd.Start()
}
