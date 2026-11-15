//go:build windows

package updater

import (
	"fmt"
	"os"
)

// ReplaceExecutable replaces targetExecPath with newBinaryPath on Windows systems.
// Windows locks running .exe from being written or deleted directly, but permits renaming it.
func ReplaceExecutable(targetExecPath, newBinaryPath string) error {
	oldPath := targetExecPath + ".old"
	_ = os.Remove(oldPath)

	// Rename running binary to .old
	if err := os.Rename(targetExecPath, oldPath); err != nil {
		// If target didn't exist yet (initial install), that's fine
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to move running executable to %s: %w", oldPath, err)
		}
	}

	// Move new binary into original target position
	err := os.Rename(newBinaryPath, targetExecPath)
	if err != nil {
		// Fallback for cross-drive/volume: copy then delete
		if copyErr := copyFile(newBinaryPath, targetExecPath, 0755); copyErr != nil {
			// Restore original binary from .old if replacement failed
			_ = os.Rename(oldPath, targetExecPath)
			return fmt.Errorf("failed to place new executable: %w", copyErr)
		}
		_ = os.Remove(newBinaryPath)
	}

	// Try to remove .old; if locked, will be cleaned up next time via CleanupOldExecutables
	_ = os.Remove(oldPath)
	return nil
}
