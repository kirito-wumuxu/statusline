//go:build !windows

package updater

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReplaceExecutable replaces targetExecPath with newBinaryPath on Unix-like systems.
// It leverages Unix inode semantics to atomically rename over the executing binary.
func ReplaceExecutable(targetExecPath, newBinaryPath string) error {
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("failed to chmod new binary: %w", err)
	}

	// First attempt atomic rename
	err := os.Rename(newBinaryPath, targetExecPath)
	if err == nil {
		return nil
	}

	// Fallback for cross-filesystem / cross-device link (EXDEV):
	// Copy to temporary file within the SAME directory as target, then atomic rename
	dir := filepath.Dir(targetExecPath)
	sameDirTmp := filepath.Join(dir, fmt.Sprintf(".%s.upgrade.tmp", filepath.Base(targetExecPath)))
	defer os.Remove(sameDirTmp)

	if copyErr := copyFile(newBinaryPath, sameDirTmp, 0755); copyErr != nil {
		return fmt.Errorf("failed to copy binary across devices: %w", copyErr)
	}

	if renameErr := os.Rename(sameDirTmp, targetExecPath); renameErr != nil {
		return fmt.Errorf("failed to replace binary: %w", renameErr)
	}

	_ = os.Remove(newBinaryPath)
	return nil
}
