package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != 8765 {
		t.Fatalf("expected default port 8765, got %d", cfg.Server.Port)
	}
	if cfg.Server.TimeoutMS != 100 {
		t.Fatalf("expected default timeout 100ms, got %d", cfg.Server.TimeoutMS)
	}
	if cfg.Server.DBPath == "" {
		t.Fatalf("expected non-empty default db path")
	}
	if len(cfg.Display.Segments) != 7 {
		t.Fatalf("expected 7 default segments, got %d", len(cfg.Display.Segments))
	}
	if cfg.Display.Segments[2] != "account" {
		t.Fatalf("expected 3rd segment to be account, got %s", cfg.Display.Segments[2])
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("STATUSLINE_SERVER_URL", "http://custom:9999/api/v1/report")
	defer os.Unsetenv("STATUSLINE_SERVER_URL")
	os.Setenv("STATUSLINE_SEGMENTS", "account,model,tokens")
	defer os.Unsetenv("STATUSLINE_SEGMENTS")

	cfg := DefaultConfig()
	if cfg.Server.Endpoint != "http://custom:9999/api/v1/report" {
		t.Fatalf("expected env override, got %s", cfg.Server.Endpoint)
	}
	if len(cfg.Display.Segments) != 3 || cfg.Display.Segments[0] != "account" {
		t.Fatalf("expected segments override, got %v", cfg.Display.Segments)
	}
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
server:
  endpoint: "http://127.0.0.1:18765/api/v1/report"
  port: 18765
  timeout_ms: 150
node:
  id: "test-node"
display:
  segments:
    - account
    - model
    - tokens
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Server.Endpoint != "http://127.0.0.1:18765/api/v1/report" {
		t.Errorf("expected endpoint override from file, got %s", cfg.Server.Endpoint)
	}
	if cfg.Server.Port != 18765 {
		t.Errorf("expected port 18765, got %d", cfg.Server.Port)
	}
	if cfg.Node.ID != "test-node" {
		t.Errorf("expected node ID test-node, got %s", cfg.Node.ID)
	}
	if len(cfg.Display.Segments) != 3 || cfg.Display.Segments[0] != "account" {
		t.Errorf("expected loaded segments [account model tokens], got %v", cfg.Display.Segments)
	}
}
