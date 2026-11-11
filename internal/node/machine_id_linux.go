//go:build linux

package node

import (
	"os"
	"strings"
)

func readPlatformMachineID() string {
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
	}

	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
