package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// GetExecutablePath returns the absolute, symlink-evaluated path of the currently running binary.
func GetExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return execPath, nil
	}
	return realPath, nil
}

// CleanupOldExecutables removes any leftover .old binary file from previous upgrades.
func CleanupOldExecutables(execPath string) {
	if execPath == "" {
		return
	}
	oldPath := execPath + ".old"
	_ = os.Remove(oldPath)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
