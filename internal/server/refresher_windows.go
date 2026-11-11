//go:build windows

package server

func defaultSignal(pid int) error {
	return nil
}

func defaultVerifyProcess(pid int) bool {
	return pid > 0
}
