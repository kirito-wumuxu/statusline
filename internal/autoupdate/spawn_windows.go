//go:build windows

package autoupdate

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnDetachedAutoUpdate spawns statusline in the background detached from the current process on Windows.
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
		CreationFlags: 0x08000000 | 0x00000200, // CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
	}
	return cmd.Start()
}
