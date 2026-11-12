package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureAntigravitySettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "sub", "settings.json")
	binPath := "/opt/bin/statusline"

	var out bytes.Buffer
	err := RunInit([]string{"-settings", settingsPath, "-bin", binPath}, &out)
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
	if statusLine["type"] != "command" {
		t.Errorf("expected type command, got %v", statusLine["type"])
	}
	if statusLine["enabled"] != true {
		t.Errorf("expected enabled true, got %v", statusLine["enabled"])
	}
}

func TestConfigureClaudeCodeSettings(t *testing.T) {
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, ".claude", "settings.json")
	binPath := "/home/user/workspace/statusline/bin/statusline"

	var out bytes.Buffer
	err := RunInit([]string{"-settings", claudePath, "-bin", binPath}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("failed to read created claude settings: %v", err)
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
	if statusLine["type"] != "command" {
		t.Errorf("expected type command, got %v", statusLine["type"])
	}
}

func TestConfigureClaudeCodeWindowsBackslashConversion(t *testing.T) {
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, ".claude", "settings.json")
	binPathWindows := `D:\Software\Statusline\statusline.exe`

	var out bytes.Buffer
	err := RunInit([]string{"-settings", claudePath, "-bin", binPathWindows}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("failed to read created claude settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	statusLine, ok := parsed["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("expected statusLine object in JSON")
	}

	expected := "D:/Software/Statusline/statusline.exe"
	if statusLine["command"] != expected {
		t.Errorf("expected command %s, got %v", expected, statusLine["command"])
	}

	if !strings.Contains(out.String(), expected) {
		t.Errorf("expected output log to display %s, got: %s", expected, out.String())
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
	err := RunInit([]string{"-settings", settingsPath, "-bin", newBin}, &out)
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
	if statusLine["type"] != "command" {
		t.Errorf("expected type command, got %v", statusLine["type"])
	}
}

func TestInPlaceBinaryResolution(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	var out bytes.Buffer
	// Without -bin, it resolves running executable directly
	err := RunInit([]string{"-settings", settingsPath}, &out)
	if err != nil {
		t.Fatalf("RunInit failed: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "Using statusline binary at:") {
		t.Errorf("expected output to state using binary at, got: %s", outStr)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	statusLine := parsed["statusLine"].(map[string]any)
	cmdVal, ok := statusLine["command"].(string)
	if !ok || cmdVal == "" {
		t.Errorf("expected non-empty command path, got %v", statusLine["command"])
	}
	if !filepath.IsAbs(cmdVal) {
		t.Errorf("expected absolute command path, got %s", cmdVal)
	}
}
