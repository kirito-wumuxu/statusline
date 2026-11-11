package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	if cfg.Report.Endpoint != "http://127.0.0.1:8765/api/v1/report" {
		t.Fatalf("expected default endpoint, got %s", cfg.Report.Endpoint)
	}
	if cfg.Report.TimeoutMS != 100 {
		t.Fatalf("expected default timeout 100ms, got %d", cfg.Report.TimeoutMS)
	}
	if len(cfg.Display.Segments) != 7 {
		t.Fatalf("expected 7 default segments, got %d", len(cfg.Display.Segments))
	}
	if cfg.Display.Segments[2] != "account" {
		t.Fatalf("expected 3rd segment to be account, got %s", cfg.Display.Segments[2])
	}
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	if cfg.Server.Port != 8765 {
		t.Fatalf("expected default port 8765, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Storage.DBPath == "" {
		t.Fatalf("expected non-empty default db path")
	}
}

func TestClientEnvOverrides(t *testing.T) {
	os.Setenv("STATUSLINE_SERVER_URL", "http://custom:9999/api/v1/report")
	defer os.Unsetenv("STATUSLINE_SERVER_URL")
	os.Setenv("STATUSLINE_SEGMENTS", "account,model,tokens")
	defer os.Unsetenv("STATUSLINE_SEGMENTS")

	cfg := DefaultClientConfig()
	if cfg.Report.Endpoint != "http://custom:9999/api/v1/report" {
		t.Fatalf("expected env override, got %s", cfg.Report.Endpoint)
	}
	if len(cfg.Display.Segments) != 3 || cfg.Display.Segments[0] != "account" {
		t.Fatalf("expected segments override, got %v", cfg.Display.Segments)
	}
}

func TestServerEnvOverrides(t *testing.T) {
	os.Setenv("STATUSLINE_PORT", "9090")
	defer os.Unsetenv("STATUSLINE_PORT")
	os.Setenv("STATUSLINE_DB_PATH", "/tmp/custom.db")
	defer os.Unsetenv("STATUSLINE_DB_PATH")

	cfg := DefaultServerConfig()
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port override, got %d", cfg.Server.Port)
	}
	if cfg.Storage.DBPath != "/tmp/custom.db" {
		t.Fatalf("expected db_path override, got %s", cfg.Storage.DBPath)
	}
}

func TestLoadClientConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
report:
  endpoint: "http://10.0.0.1:8765/api/v1/report"
  timeout_ms: 150
node:
  id: "laptop-01"
display:
  segments:
    - account
    - model
    - tokens
`
	configPath := filepath.Join(tmpDir, "client.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadClientConfig(configPath)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}
	if cfg.Report.Endpoint != "http://10.0.0.1:8765/api/v1/report" {
		t.Errorf("expected endpoint override from file, got %s", cfg.Report.Endpoint)
	}
	if cfg.Report.TimeoutMS != 150 {
		t.Errorf("expected timeout 150, got %d", cfg.Report.TimeoutMS)
	}
	if cfg.Node.ID != "laptop-01" {
		t.Errorf("expected node ID laptop-01, got %s", cfg.Node.ID)
	}
	if len(cfg.Display.Segments) != 3 || cfg.Display.Segments[0] != "account" {
		t.Errorf("expected loaded segments, got %v", cfg.Display.Segments)
	}
}

func TestLoadServerConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
server:
  host: "127.0.0.1"
  port: 8888
storage:
  db_path: "/data/statusline.db"
  wal_mode: true
`
	configPath := filepath.Join(tmpDir, "server.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadServerConfig(configPath)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8888 {
		t.Errorf("expected port 8888, got %d", cfg.Server.Port)
	}
	if cfg.Storage.DBPath != "/data/statusline.db" {
		t.Errorf("expected db_path, got %s", cfg.Storage.DBPath)
	}
}

func TestLegacyConfigYamlCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	legacyYAML := `
server:
  endpoint: "http://127.0.0.1:8765/api/v1/report"
  port: 7777
  db_path: "/legacy/statusline.db"
node:
  id: "legacy-node"
display:
  segments:
    - dir
    - model
`
	legacyPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(legacyPath, []byte(legacyYAML), 0644); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	// 1. Client loads legacy config.yaml
	clientCfg, err := LoadClientConfig(legacyPath)
	if err != nil {
		t.Fatalf("LoadClientConfig on legacy config failed: %v", err)
	}
	if clientCfg.Report.Endpoint != "http://127.0.0.1:8765/api/v1/report" {
		t.Errorf("expected client to read server.endpoint from legacy config, got %s", clientCfg.Report.Endpoint)
	}
	if clientCfg.Node.ID != "legacy-node" {
		t.Errorf("expected node ID legacy-node, got %s", clientCfg.Node.ID)
	}

	// 2. Server loads legacy config.yaml
	serverCfg, err := LoadServerConfig(legacyPath)
	if err != nil {
		t.Fatalf("LoadServerConfig on legacy config failed: %v", err)
	}
	if serverCfg.Server.Port != 7777 {
		t.Errorf("expected server to read server.port from legacy config, got %d", serverCfg.Server.Port)
	}
	if serverCfg.Storage.DBPath != "/legacy/statusline.db" {
		t.Errorf("expected server to read server.db_path from legacy config, got %s", serverCfg.Storage.DBPath)
	}
}
