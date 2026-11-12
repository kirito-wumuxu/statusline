package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	if cfg.Report.Endpoint != "http://127.0.0.1:8628/api/v1/report" {
		t.Fatalf("expected default endpoint http://127.0.0.1:8628/api/v1/report, got %s", cfg.Report.Endpoint)
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
	if cfg.Server.Port != 8628 {
		t.Fatalf("expected default port 8628, got %d", cfg.Server.Port)
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
  endpoint: "http://10.0.0.1:8628/api/v1/report"
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
	if cfg.Report.Endpoint != "http://10.0.0.1:8628/api/v1/report" {
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

func TestBinaryDirConfigResolution(t *testing.T) {
	// Verify that getExeDir returns a valid directory path or empty string
	exeDir := getExeDir()
	if exeDir != "" {
		if _, err := os.Stat(exeDir); err != nil {
			t.Errorf("getExeDir returned non-existent directory: %s", exeDir)
		}
	}

	// Test loading config automatically from executable directory
	tmpDir := t.TempDir()
	origGetExeDir := getExeDir
	getExeDir = func() string { return tmpDir }
	defer func() { getExeDir = origGetExeDir }()

	clientYAML := `
report:
  endpoint: "http://127.0.0.1:8628/api/v1/report"
node:
  id: "exe-dir-node"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "client.yaml"), []byte(clientYAML), 0644); err != nil {
		t.Fatalf("failed to write client.yaml: %v", err)
	}

	serverYAML := `
server:
  port: 8628
`
	if err := os.WriteFile(filepath.Join(tmpDir, "server.yaml"), []byte(serverYAML), 0644); err != nil {
		t.Fatalf("failed to write server.yaml: %v", err)
	}

	clientCfg, err := LoadClientConfig("")
	if err != nil {
		t.Fatalf("LoadClientConfig from exe dir failed: %v", err)
	}
	if clientCfg.Node.ID != "exe-dir-node" {
		t.Errorf("expected node id 'exe-dir-node', got %s", clientCfg.Node.ID)
	}

	serverCfg, err := LoadServerConfig("")
	if err != nil {
		t.Fatalf("LoadServerConfig from exe dir failed: %v", err)
	}
	if serverCfg.Server.Port != 8628 {
		t.Errorf("expected port 8628, got %d", serverCfg.Server.Port)
	}
}
