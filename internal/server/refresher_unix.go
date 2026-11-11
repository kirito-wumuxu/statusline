//go:build !windows

package server

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func defaultSignal(pid int) error {
	return syscall.Kill(pid, syscall.SIGWINCH)
}

func defaultVerifyProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	commBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err == nil {
		comm := strings.ToLower(strings.TrimSpace(string(commBytes)))
		if strings.Contains(comm, "agy") || strings.Contains(comm, "claude") {
			return true
		}
	}
	cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err == nil {
		cmd := strings.ToLower(string(cmdlineBytes))
		return strings.Contains(cmd, "agy") || strings.Contains(cmd, "claude")
	}
	return true
}
