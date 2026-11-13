package installer

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"statusline/internal/config"
	"statusline/internal/model"
)

// Options holds configuration for statusline initialization.
type Options struct {
	Target       string
	SettingsPath string
	BinPath      string
}

// RunInit executes the `statusline init` command.
func RunInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(out)

	var opts Options
	fs.StringVar(&opts.Target, "target", "all", "Target CLI to configure: all, agy (antigravity), or claude (claude code)")
	fs.StringVar(&opts.SettingsPath, "settings", "", "Custom path to settings.json")
	fs.StringVar(&opts.BinPath, "bin", "", "Custom binary path to configure")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 1. Resolve running binary path directly in-place without moving or copying
	targetBin := opts.BinPath
	if targetBin == "" {
		currentExe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("unable to determine binary path: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
			currentExe = resolved
		}
		if abs, err := filepath.Abs(currentExe); err == nil {
			currentExe = abs
		}
		targetBin = currentExe
	}
	fmt.Fprintf(out, "✔ Using statusline binary at: %s\n", targetBin)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to determine home directory: %w", err)
	}

	target := strings.ToLower(strings.TrimSpace(opts.Target))

	// If a custom settings path is provided, configure it directly
	if opts.SettingsPath != "" {
		isClaude := strings.Contains(strings.ToLower(opts.SettingsPath), "claude")
		cmdBin := targetBin
		if isClaude {
			cmdBin = strings.ReplaceAll(targetBin, "\\", "/")
		}
		if err := configureSettingsFile(opts.SettingsPath, cmdBin, isClaude); err != nil {
			return fmt.Errorf("failed to configure %s: %w", opts.SettingsPath, err)
		}
		label := "Antigravity"
		if isClaude {
			label = "Claude Code"
		}
		fmt.Fprintf(out, "✔ Configured %s settings: %s\n", label, opts.SettingsPath)
		fmt.Fprintf(out, "  - statusLine.command: %s\n", cmdBin)
		SyncAliasesFromRemote(out)
		fmt.Fprintln(out, "🎉 Statusline initialized successfully!")
		return nil
	}

	configuredCount := 0

	// 2. Configure Antigravity CLI if target is "all" or "agy"
	if target == "all" || target == "agy" || target == "antigravity" {
		agyPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "settings.json")
		geminiPath := filepath.Join(homeDir, ".gemini", "settings.json")

		targetFile := agyPath
		if _, err := os.Stat(agyPath); err != nil {
			if _, err2 := os.Stat(geminiPath); err2 == nil {
				targetFile = geminiPath
			}
		}

		if err := configureSettingsFile(targetFile, targetBin, false); err != nil {
			return fmt.Errorf("failed to configure Antigravity settings %s: %w", targetFile, err)
		}
		fmt.Fprintf(out, "✔ Configured Antigravity settings: %s\n", targetFile)
		fmt.Fprintf(out, "  - statusLine.type: command\n")
		fmt.Fprintf(out, "  - statusLine.command: %s\n", targetBin)
		fmt.Fprintf(out, "  - statusLine.enabled: true\n")
		configuredCount++
	}

	// 3. Configure Claude Code if target is "all" or "claude"
	if target == "all" || target == "claude" || target == "claudecode" {
		claudeBin := strings.ReplaceAll(targetBin, "\\", "/")
		claudePath := filepath.Join(homeDir, ".claude", "settings.json")
		if err := configureSettingsFile(claudePath, claudeBin, true); err != nil {
			return fmt.Errorf("failed to configure Claude Code settings %s: %w", claudePath, err)
		}
		fmt.Fprintf(out, "✔ Configured Claude Code settings: %s\n", claudePath)
		fmt.Fprintf(out, "  - statusLine.type: command\n")
		fmt.Fprintf(out, "  - statusLine.command: %s\n", claudeBin)
		configuredCount++

		// Check if current workspace has a local .claude directory
		cwd, _ := os.Getwd()
		localClaudeDir := filepath.Join(cwd, ".claude")
		if fi, err := os.Stat(localClaudeDir); err == nil && fi.IsDir() {
			localClaudePath := filepath.Join(localClaudeDir, "settings.json")
			if err := configureSettingsFile(localClaudePath, claudeBin, true); err == nil {
				fmt.Fprintf(out, "✔ Configured local workspace Claude Code settings: %s\n", localClaudePath)
			}
		}
	}

	if configuredCount > 0 {
		SyncAliasesFromRemote(out)
		fmt.Fprintln(out, "🎉 Statusline initialized successfully! Restart your AI CLI (Antigravity / Claude Code) to apply changes.")
	}

	return nil
}

func configureSettingsFile(filePath, binPath string, isClaude bool) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	settings := make(map[string]any)

	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			_ = os.WriteFile(filePath+".bak", data, 0644)
			settings = make(map[string]any)
		}
	}

	var statusLine map[string]any
	if existing, ok := settings["statusLine"].(map[string]any); ok {
		statusLine = existing
	} else {
		statusLine = make(map[string]any)
	}

	cmd := binPath
	if isClaude {
		cmd = strings.ReplaceAll(binPath, "\\", "/")
	}

	statusLine["type"] = "command"
	statusLine["command"] = cmd
	if !isClaude {
		statusLine["enabled"] = true
	}

	settings["statusLine"] = statusLine

	formatted, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')

	return os.WriteFile(filePath, formatted, 0644)
}

// SyncAliasesFromRemote attempts to fetch latest display aliases from statusline-server and merge into client.yaml.
func SyncAliasesFromRemote(out io.Writer) {
	cfg, _ := config.LoadClientConfig("")
	if cfg == nil || cfg.Report.Endpoint == "" || strings.ToLower(cfg.Report.Endpoint) == "off" {
		return
	}

	endpoint := cfg.Report.Endpoint
	aliasesURL := endpoint
	if idx := strings.Index(endpoint, "/api/v1/report"); idx != -1 {
		aliasesURL = endpoint[:idx] + "/api/v1/config/aliases"
	} else if idx := strings.Index(endpoint, "/api/report"); idx != -1 {
		aliasesURL = endpoint[:idx] + "/api/config/aliases"
	} else {
		aliasesURL = strings.TrimRight(endpoint, "/") + "/api/v1/config/aliases"
	}

	req, err := http.NewRequest(http.MethodGet, aliasesURL, nil)
	if err != nil {
		return
	}
	if cfg.Report.APIKey != "" {
		req.Header.Set("X-API-Key", cfg.Report.APIKey)
		req.Header.Set("Authorization", "Bearer "+cfg.Report.APIKey)
	}

	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(out, "ℹ 未连接到 statusline-server (%s)，跳过别名同步\n", aliasesURL)
		return
	}
	defer resp.Body.Close()

	var remoteAliases model.DisplayAliasesConfig
	if err := json.NewDecoder(resp.Body).Decode(&remoteAliases); err != nil {
		return
	}

	// Update local client.yaml
	clientYamlPath := "client.yaml"
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exeDir := filepath.Dir(resolved)
			targetInExeDir := filepath.Join(exeDir, "client.yaml")
			if _, err := os.Stat(targetInExeDir); err == nil {
				clientYamlPath = targetInExeDir
			}
		}
	}

	// Read existing or create new
	var fileCfg config.ClientConfig
	if data, err := os.ReadFile(clientYamlPath); err == nil {
		_ = yaml.Unmarshal(data, &fileCfg)
	} else {
		fileCfg = *cfg
	}

	fileCfg.Aliases = config.DisplayAliasesConfig{
		Quotas:   make([]config.DisplayAliasItem, 0, len(remoteAliases.Quotas)),
		Models:   make([]config.DisplayAliasItem, 0, len(remoteAliases.Models)),
		Accounts: make([]config.DisplayAliasItem, 0, len(remoteAliases.Accounts)),
	}

	for _, q := range remoteAliases.Quotas {
		fileCfg.Aliases.Quotas = append(fileCfg.Aliases.Quotas, config.DisplayAliasItem{
			Key:       q.Key,
			Name:      q.DisplayName,
			ShortName: q.ShortName,
		})
	}
	for _, m := range remoteAliases.Models {
		fileCfg.Aliases.Models = append(fileCfg.Aliases.Models, config.DisplayAliasItem{
			Key:       m.Key,
			Name:      m.DisplayName,
			ShortName: m.ShortName,
		})
	}
	for _, a := range remoteAliases.Accounts {
		fileCfg.Aliases.Accounts = append(fileCfg.Aliases.Accounts, config.DisplayAliasItem{
			Key:       a.Key,
			Name:      a.DisplayName,
			ShortName: a.ShortName,
			SortOrder: a.SortOrder,
		})
	}

	outBytes, err := yaml.Marshal(&fileCfg)
	if err == nil {
		_ = os.WriteFile(clientYamlPath, outBytes, 0644)
		fmt.Fprintf(out, "✔ 成功从服务端同步别名映射并写入 %s: %d 个配额规则, %d 个模型规则, %d 个账号规则\n",
			clientYamlPath, len(fileCfg.Aliases.Quotas), len(fileCfg.Aliases.Models), len(fileCfg.Aliases.Accounts))
	}
}
