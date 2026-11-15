package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "statusline")
	_ = os.WriteFile(origPath, []byte("v1 content"), 0755)

	newPath := filepath.Join(tmpDir, "statusline.upgrade.tmp")
	_ = os.WriteFile(newPath, []byte("v2 content"), 0755)

	if err := ReplaceExecutable(origPath, newPath); err != nil {
		t.Fatalf("ReplaceExecutable failed: %v", err)
	}

	content, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read replaced file: %v", err)
	}
	if string(content) != "v2 content" {
		t.Errorf("expected v2 content, got %s", string(content))
	}

	// Verify temp file is consumed / cleaned up
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be consumed or removed")
	}
}

func TestCleanupOldExecutables(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "statusline")
	_ = os.WriteFile(origPath, []byte("v2 content"), 0755)
	oldPath := origPath + ".old"
	_ = os.WriteFile(oldPath, []byte("v1 old content"), 0755)

	CleanupOldExecutables(origPath)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected .old file to be cleaned up")
	}
}
