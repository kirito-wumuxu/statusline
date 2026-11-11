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
	Server  ReportConfig  `yaml:"server"` // legacy alias for report in config.yaml
	Node    NodeConfig    `yaml:"node"`
	Display DisplayConfig `yaml:"display"`
}

// Config is an alias to ClientConfig for backwards compatibility.
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
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key"`
	Endpoint  string `yaml:"endpoint"` // legacy fallback
	TimeoutMS int    `yaml:"timeout_ms"`
	DBPath    string `yaml:"db_path"` // legacy fallback from config.yaml
}

type StorageConfig struct {
	DBPath  string `yaml:"db_path"`
	WALMode bool   `yaml:"wal_mode"`
}

func DefaultSegments() []string {
	return []string{"dir", "vcs", "account", "model", "context", "tokens", "quota"}
}

func DefaultClientConfig() *ClientConfig {
	cfg := &ClientConfig{
		Report: ReportConfig{
			Endpoint:  "http://127.0.0.1:8765/api/v1/report",
			TimeoutMS: 100,
			APIKey:    "",
		},
		Server: ReportConfig{
			Endpoint:  "http://127.0.0.1:8765/api/v1/report",
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
			Port:   8765,
			APIKey: "",
			DBPath: defaultDB,
		},
		Storage: StorageConfig{
			DBPath:  defaultDB,
			WALMode: true,
		},
	}
	applyServerEnvOverrides(cfg)
	return cfg
}

// LoadClientConfig loads client settings from client.yaml, ~/.config/statusline/client.yaml, or config.yaml.
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{
		Node: NodeConfig{
			ID:         "",
			CustomTags: map[string]string{"env": "local"},
		},
		Display: DisplayConfig{
			EnableStdout: true,
			Segments:     DefaultSegments(),
		},
	}

	if path == "" {
		homeDir, _ := os.UserHomeDir()
		candidates := []string{
			os.Getenv("STATUSLINE_CLIENT_CONFIG"),
			os.Getenv("STATUSLINE_CONFIG"),
			"client.yaml",
			filepath.Join(homeDir, ".config", "statusline", "client.yaml"),
			"config.yaml",
			filepath.Join(homeDir, ".config", "statusline", "config.yaml"),
		}
		for _, c := range candidates {
			if c != "" {
				expanded := expandHome(c)
				if _, err := os.Stat(expanded); err == nil {
					path = expanded
					break
				}
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

	// Resolve endpoint defaults and legacy fallbacks
	if cfg.Report.Endpoint == "" {
		if cfg.Server.Endpoint != "" {
			cfg.Report.Endpoint = cfg.Server.Endpoint
		} else {
			cfg.Report.Endpoint = "http://127.0.0.1:8765/api/v1/report"
		}
	}
	if cfg.Report.TimeoutMS == 0 {
		if cfg.Server.TimeoutMS != 0 {
			cfg.Report.TimeoutMS = cfg.Server.TimeoutMS
		} else {
			cfg.Report.TimeoutMS = 100
		}
	}
	if cfg.Report.APIKey == "" && cfg.Server.APIKey != "" {
		cfg.Report.APIKey = cfg.Server.APIKey
	}

	// Mirror to Server field for full backward compatibility
	cfg.Server.Endpoint = cfg.Report.Endpoint
	cfg.Server.TimeoutMS = cfg.Report.TimeoutMS
	cfg.Server.APIKey = cfg.Report.APIKey

	if len(cfg.Display.Segments) == 0 {
		cfg.Display.Segments = DefaultSegments()
	}

	applyClientEnvOverrides(cfg)
	return cfg, nil
}

func LoadConfig(path string) (*ClientConfig, error) {
	return LoadClientConfig(path)
}

// LoadServerConfig loads server settings from server.yaml, ~/.config/statusline/server.yaml, or config.yaml.
func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := &ServerConfig{
		Server: ServerListenConfig{
			Host:   "0.0.0.0",
			Port:   8765,
			APIKey: "",
		},
		Storage: StorageConfig{
			WALMode: true,
		},
	}

	if path == "" {
		homeDir, _ := os.UserHomeDir()
		candidates := []string{
			os.Getenv("STATUSLINE_SERVER_CONFIG"),
			os.Getenv("STATUSLINE_CONFIG"),
			"server.yaml",
			filepath.Join(homeDir, ".config", "statusline", "server.yaml"),
			"config.yaml",
			filepath.Join(homeDir, ".config", "statusline", "config.yaml"),
		}
		for _, c := range candidates {
			if c != "" {
				expanded := expandHome(c)
				if _, err := os.Stat(expanded); err == nil {
					path = expanded
					break
				}
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

	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".local", "share", "statusline", "statusline.db")

	// Legacy fallback: if storage.db_path is empty but server.db_path is set in config.yaml
	if cfg.Storage.DBPath == "" {
		if cfg.Server.DBPath != "" {
			cfg.Storage.DBPath = cfg.Server.DBPath
		} else {
			cfg.Storage.DBPath = defaultDB
		}
	}
	cfg.Server.DBPath = cfg.Storage.DBPath

	applyServerEnvOverrides(cfg)
	cfg.Storage.DBPath = expandHome(cfg.Storage.DBPath)
	cfg.Server.DBPath = cfg.Storage.DBPath
	return cfg, nil
}

func applyClientEnvOverrides(cfg *ClientConfig) {
	if endpoint := os.Getenv("STATUSLINE_SERVER_URL"); endpoint != "" {
		cfg.Report.Endpoint = endpoint
		cfg.Server.Endpoint = endpoint
	}
	if timeoutStr := os.Getenv("STATUSLINE_TIMEOUT_MS"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			cfg.Report.TimeoutMS = t
			cfg.Server.TimeoutMS = t
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
		cfg.Server.DBPath = dbPath
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
