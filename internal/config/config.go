package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Node    NodeConfig    `yaml:"node"`
	Display DisplayConfig `yaml:"display"`
}

type ServerConfig struct {
	Endpoint  string `yaml:"endpoint"`
	APIKey    string `yaml:"api_key"`
	TimeoutMS int    `yaml:"timeout_ms"`
	Port      int    `yaml:"port"`
	DBPath    string `yaml:"db_path"`
}

type NodeConfig struct {
	ID         string            `yaml:"id"`
	CustomTags map[string]string `yaml:"custom_tags"`
}

type DisplayConfig struct {
	EnableStdout bool   `yaml:"enable_stdout"`
	Format       string `yaml:"format"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".local", "share", "statusline", "statusline.db")

	cfg := &Config{
		Server: ServerConfig{
			Endpoint:  "http://127.0.0.1:8765/api/v1/report",
			APIKey:    "",
			TimeoutMS: 100,
			Port:      8765,
			DBPath:    defaultDB,
		},
		Node: NodeConfig{
			ID:         "",
			CustomTags: map[string]string{"env": "local"},
		},
		Display: DisplayConfig{
			EnableStdout: true,
			Format:       "[{{.Product}}] {{.Model}} | Ctx: {{.UsedPct}}% | {{.Tokens}}",
		},
	}

	applyEnvOverrides(cfg)
	return cfg
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		if envPath := os.Getenv("STATUSLINE_CONFIG"); envPath != "" {
			path = envPath
		} else if _, err := os.Stat("config.yaml"); err == nil {
			path = "config.yaml"
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

	applyEnvOverrides(cfg)
	cfg.Server.DBPath = expandHome(cfg.Server.DBPath)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if endpoint := os.Getenv("STATUSLINE_SERVER_URL"); endpoint != "" {
		cfg.Server.Endpoint = endpoint
	}
	if nodeID := os.Getenv("STATUSLINE_NODE_ID"); nodeID != "" {
		cfg.Node.ID = nodeID
	}
	if dbPath := os.Getenv("STATUSLINE_DB_PATH"); dbPath != "" {
		cfg.Server.DBPath = dbPath
	}
	if portStr := os.Getenv("STATUSLINE_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(homeDir, path[1:])
		}
	}
	return path
}
