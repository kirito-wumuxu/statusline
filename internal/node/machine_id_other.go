//go:build !linux && !windows

package node

func readPlatformMachineID() string {
	return ""
}
