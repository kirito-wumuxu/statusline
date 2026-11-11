package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureNewSettingsFile(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "sub", "settings.json")
	binPath := "/opt/bin/statusline"

	var out bytes.Buffer
	err := RunInit([]string{"-settings", settingsPath, "-bin", binPath, "-no-copy"}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read created settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	statusLine, ok := parsed["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("expected statusLine object in JSON")
	}

	if statusLine["command"] != binPath {
		t.Errorf("expected command %s, got %v", binPath, statusLine["command"])
	}
	if statusLine["enabled"] != true {
		t.Errorf("expected enabled true, got %v", statusLine["enabled"])
	}
	if !strings.Contains(out.String(), "Statusline initialized successfully") {
		t.Errorf("expected success message in output, got: %s", out.String())
	}
}

func TestConfigureExistingSettingsPreservesFields(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	initialJSON := `{
  "model": "Gemini 3.8 Flash (High)",
  "trustedWorkspaces": ["/home/user"],
  "verbosity": "low",
  "statusLine": {
    "type": "custom",
    "command": "/old/path/agy-statusline",
    "enabled": false
  }
}`
	if err := os.WriteFile(settingsPath, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial settings: %v", err)
	}

	newBin := "/new/path/statusline"
	var out bytes.Buffer
	err := RunInit([]string{"-settings", settingsPath, "-bin", newBin, "-no-copy"}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Verify existing fields preserved
	if parsed["model"] != "Gemini 3.8 Flash (High)" {
		t.Errorf("expected model preserved, got %v", parsed["model"])
	}
	if parsed["verbosity"] != "low" {
		t.Errorf("expected verbosity preserved, got %v", parsed["verbosity"])
	}

	// Verify statusLine updated
	statusLine := parsed["statusLine"].(map[string]any)
	if statusLine["command"] != newBin {
		t.Errorf("expected new command %s, got %v", newBin, statusLine["command"])
	}
	if statusLine["enabled"] != true {
		t.Errorf("expected enabled true, got %v", statusLine["enabled"])
	}
	if statusLine["type"] != "custom" {
		t.Errorf("expected type custom preserved, got %v", statusLine["type"])
	}
}
