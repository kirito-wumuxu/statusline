package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClientConfig holds configuration for the statusline client plugin.
type ClientConfig struct {
	Report  ReportConfig  `yaml:"report"`
	Node    NodeConfig    `yaml:"node"`
	Display DisplayConfig `yaml:"display"`
}

// Config is an alias to ClientConfig.
type Config = ClientConfig

type ReportConfig struct {
	Endpoint  string `yaml:"endpoint"`
	TimeoutMS int    `yaml:"timeout_ms"`
	APIKey    string `yaml:"api_key"`
}

type NodeConfig struct {
	ID         string            `yaml:"id"`
	CustomTags map[string]string `yaml:"custom_tags"`
}

type DisplayConfig struct {
	EnableStdout bool     `yaml:"enable_stdout"`
	Format       string   `yaml:"format"`
	Segments     []string `yaml:"segments"`
}

// ServerConfig holds configuration for the statusline-server telemetry daemon.
type ServerConfig struct {
	Server  ServerListenConfig `yaml:"server"`
	Storage StorageConfig      `yaml:"storage"`
}

type ServerListenConfig struct {
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	APIKey string `yaml:"api_key"`
}

type StorageConfig struct {
	DBPath  string `yaml:"db_path"`
	WALMode bool   `yaml:"wal_mode"`
}

func DefaultSegments() []string {
	return []string{"dir", "vcs", "account", "model", "context", "tokens", "quota"}
}

// getExeDir returns the absolute directory containing the current executable, resolving symlinks.
var getExeDir = defaultGetExeDir

func defaultGetExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

func DefaultClientConfig() *ClientConfig {
	cfg := &ClientConfig{
		Report: ReportConfig{
			Endpoint:  "http://127.0.0.1:8628/api/v1/report",
			TimeoutMS: 100,
			APIKey:    "",
		},
		Node: NodeConfig{
			ID:         "",
			CustomTags: map[string]string{"env": "local"},
		},
		Display: DisplayConfig{
			EnableStdout: true,
			Segments:     DefaultSegments(),
		},
	}
	applyClientEnvOverrides(cfg)
	return cfg
}

func DefaultConfig() *ClientConfig {
	return DefaultClientConfig()
}

func DefaultServerConfig() *ServerConfig {
	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".local", "share", "statusline", "statusline.db")

	cfg := &ServerConfig{
		Server: ServerListenConfig{
			Host:   "0.0.0.0",
			Port:   8628,
			APIKey: "",
		},
		Storage: StorageConfig{
			DBPath:  defaultDB,
			WALMode: true,
		},
	}
	applyServerEnvOverrides(cfg)
	return cfg
}

// LoadClientConfig loads client settings from client.yaml.
// Priority:
// 1. Explicit path parameter
// 2. STATUSLINE_CLIENT_CONFIG / STATUSLINE_CONFIG environment variable
// 3. client.yaml in the executable's directory
// 4. client.yaml in the current working directory
// 5. ~/.config/statusline/client.yaml
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := DefaultClientConfig()

	if path == "" {
		homeDir, _ := os.UserHomeDir()
		var candidates []string
		if env := os.Getenv("STATUSLINE_CLIENT_CONFIG"); env != "" {
			candidates = append(candidates, env)
		}
		if env := os.Getenv("STATUSLINE_CONFIG"); env != "" {
			candidates = append(candidates, env)
		}
		if exeDir := getExeDir(); exeDir != "" {
			candidates = append(candidates, filepath.Join(exeDir, "client.yaml"))
		}
		candidates = append(candidates, "client.yaml")
		if homeDir != "" {
			candidates = append(candidates, filepath.Join(homeDir, ".config", "statusline", "client.yaml"))
		}

		for _, c := range candidates {
			expanded := expandHome(c)
			if _, err := os.Stat(expanded); err == nil {
				path = expanded
				break
			}
		}
	}

	if path != "" {
		expanded := expandHome(path)
		data, err := os.ReadFile(expanded)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(data) > 0 {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
	}

	if cfg.Report.Endpoint == "" {
		cfg.Report.Endpoint = "http://127.0.0.1:8628/api/v1/report"
	}
	if cfg.Report.TimeoutMS <= 0 {
		cfg.Report.TimeoutMS = 100
	}
	if len(cfg.Display.Segments) == 0 {
		cfg.Display.Segments = DefaultSegments()
	}

	applyClientEnvOverrides(cfg)
	return cfg, nil
}

func LoadConfig(path string) (*ClientConfig, error) {
	return LoadClientConfig(path)
}

// LoadServerConfig loads server settings from server.yaml.
// Priority:
// 1. Explicit path parameter (-config)
// 2. STATUSLINE_SERVER_CONFIG / STATUSLINE_CONFIG environment variable
// 3. server.yaml in the executable's directory
// 4. server.yaml in the current working directory
// 5. ~/.config/statusline/server.yaml
func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := DefaultServerConfig()

	if path == "" {
		homeDir, _ := os.UserHomeDir()
		var candidates []string
		if env := os.Getenv("STATUSLINE_SERVER_CONFIG"); env != "" {
			candidates = append(candidates, env)
		}
		if env := os.Getenv("STATUSLINE_CONFIG"); env != "" {
			candidates = append(candidates, env)
		}
		if exeDir := getExeDir(); exeDir != "" {
			candidates = append(candidates, filepath.Join(exeDir, "server.yaml"))
		}
		candidates = append(candidates, "server.yaml")
		if homeDir != "" {
			candidates = append(candidates, filepath.Join(homeDir, ".config", "statusline", "server.yaml"))
		}

		for _, c := range candidates {
			expanded := expandHome(c)
			if _, err := os.Stat(expanded); err == nil {
				path = expanded
				break
			}
		}
	}

	if path != "" {
		expanded := expandHome(path)
		data, err := os.ReadFile(expanded)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(data) > 0 {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
	}

	if cfg.Server.Port <= 0 {
		cfg.Server.Port = 8628
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Storage.DBPath == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.Storage.DBPath = filepath.Join(homeDir, ".local", "share", "statusline", "statusline.db")
	}

	applyServerEnvOverrides(cfg)
	cfg.Storage.DBPath = expandHome(cfg.Storage.DBPath)
	return cfg, nil
}

func applyClientEnvOverrides(cfg *ClientConfig) {
	if endpoint := os.Getenv("STATUSLINE_SERVER_URL"); endpoint != "" {
		cfg.Report.Endpoint = endpoint
	}
	if timeoutStr := os.Getenv("STATUSLINE_TIMEOUT_MS"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			cfg.Report.TimeoutMS = t
		}
	}
	if nodeID := os.Getenv("STATUSLINE_NODE_ID"); nodeID != "" {
		cfg.Node.ID = nodeID
	}
	if segStr := os.Getenv("STATUSLINE_SEGMENTS"); segStr != "" {
		var segs []string
		for _, s := range strings.Split(segStr, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				segs = append(segs, trimmed)
			}
		}
		if len(segs) > 0 {
			cfg.Display.Segments = segs
		}
	}
}

func applyServerEnvOverrides(cfg *ServerConfig) {
	if host := os.Getenv("STATUSLINE_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if portStr := os.Getenv("STATUSLINE_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if dbPath := os.Getenv("STATUSLINE_DB_PATH"); dbPath != "" {
		cfg.Storage.DBPath = dbPath
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return homeDir
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}
