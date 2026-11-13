package installer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestSyncAliasesFromRemote(t *testing.T) {
	// Create mock server
	aliasesJSON := `{
		"quotas": [{"key": "gemini", "name": "Gemini", "short_name": "G"}],
		"models": [{"key": "gemini-3.8-flash", "name": "Gemini 3.8 Flash", "short_name": "Flash"}],
		"accounts": [{"key": "test@example.com", "name": "Tester", "short_name": "Test"}]
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/config/aliases") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(aliasesJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Temp dir for client.yaml
	tmpDir := t.TempDir()
	originalWD, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(originalWD)

	clientYaml := filepath.Join(tmpDir, "client.yaml")
	initialContent := "report:\n  endpoint: " + ts.URL + "/api/v1/report\n"
	_ = os.WriteFile(clientYaml, []byte(initialContent), 0644)

	var out bytes.Buffer
	SyncAliasesFromRemote(&out)

	outStr := out.String()
	if !strings.Contains(outStr, "成功从服务端同步别名映射") {
		t.Errorf("expected sync success message, got: %s", outStr)
	}

	data, err := os.ReadFile(clientYaml)
	if err != nil {
		t.Fatalf("failed to read updated client.yaml: %v", err)
	}
	if !strings.Contains(string(data), "short_name: Flash") {
		t.Errorf("expected client.yaml to contain updated aliases, got: %s", string(data))
	}
}
