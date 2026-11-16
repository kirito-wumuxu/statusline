package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"statusline/internal/autoupdate"
	"statusline/internal/config"
	"statusline/internal/installer"
	"statusline/internal/logging"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/reporter"
	"statusline/internal/storage"
	"statusline/internal/transcript"
	"statusline/internal/updater"
	"statusline/internal/vcs"
)

var Version = "dev"

func printHelp() {
	displayVer := Version
	if !strings.HasPrefix(displayVer, "v") && !strings.HasPrefix(displayVer, "V") && displayVer != "dev" {
		displayVer = "v" + displayVer
	}
	fmt.Printf(`statusline %s - Terminal statusline plugin for Antigravity & Claude Code

Usage:
  statusline                    Render statusline from stdin JSON (called by AI CLI)
  statusline auto-update [opts] Sync display aliases & check auto-upgrade
  statusline sync [options]     Scan local transcripts and sync missing turns to server
  statusline init [options]     Auto-configure settings.json for Antigravity & Claude Code
  statusline upgrade [options]  Self-upgrade statusline to the latest release
  statusline update [options]   Alias for upgrade
  statusline --version, -v      Print version information
  statusline --help, -h         Show this help message

Auto-Update Options:
  -force                        Force sync & update even if already executed today
  -check                        Check for updates without modifying files

Sync Options:
  -home <path>                  Custom home directory to scan transcripts from

Upgrade Options:
  -check                        Check for new version without downloading or installing
  -all                          Also upgrade statusline-server if present in same directory
  -version <tag>                Target version tag to install (default: latest)
  -source <auto|server|github>  Update source (default: auto)
  -y, --yes                     Skip confirmation prompts

Init Options:
  -target <all|agy|claude>      Target CLI to configure (default: all)
  -settings <path>              Specify custom path to settings.json
  -bin <path>                   Specify custom binary path (default: in-place binary location)
`, Version)
}

func runAutoUpdateCommand(args []string, cfg *config.Config, out io.Writer) error {
	fs := flag.NewFlagSet("auto-update", flag.ContinueOnError)
	fs.SetOutput(out)
	forceFlag := fs.Bool("force", false, "Force sync and update check even if already executed today")
	checkFlag := fs.Bool("check", false, "Check for updates without modifying files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return autoupdate.RunAutoUpdate(cfg, Version, *forceFlag, *checkFlag, out)
}

func runUpgradeCommand(args []string, out io.Writer) error {
	return updater.RunUpgrade(args, Version, out)
}

func runSyncCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(out)
	homeFlag := fs.String("home", "", "Custom home directory to scan transcripts from")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		cfg = config.DefaultConfig()
	}

	endpoint := cfg.Report.Endpoint
	if endpoint == "" || endpoint == "off" || endpoint == "none" {
		return fmt.Errorf("上报已停用 (endpoint: %q)，无法同步到远程服务端。请在 client.yaml 中配置 report.endpoint", endpoint)
	}

	syncEndpoint := endpoint
	if strings.HasSuffix(syncEndpoint, "/api/v1/report") {
		syncEndpoint = strings.TrimSuffix(syncEndpoint, "/api/v1/report") + "/api/v1/sync/events"
	} else if strings.HasSuffix(syncEndpoint, "/api/telemetry") {
		syncEndpoint = strings.TrimSuffix(syncEndpoint, "/api/telemetry") + "/api/v1/sync/events"
	} else if strings.HasSuffix(syncEndpoint, "/") {
		syncEndpoint = syncEndpoint + "api/v1/sync/events"
	} else {
		syncEndpoint = syncEndpoint + "/api/v1/sync/events"
	}

	n := node.GetNodeInfo(cfg.Node.ID, Version)
	fmt.Fprintf(out, "[1/3] 正在扫描本地对话转录日志 (Antigravity & Claude Code)...\n")

	summary, err := transcript.DiscoverAndScanTranscripts(*homeFlag, n.NodeID)
	if err != nil {
		return fmt.Errorf("扫描本地转录失败: %w", err)
	}

	fmt.Fprintf(out, "      ✓ 扫描到 %d 个转录文件，共提取出 %d 条历史交互轮次\n", summary.FilesScanned, summary.TotalEvents)
	if summary.TotalEvents == 0 {
		fmt.Fprintf(out, "[2/3] 没有发现需要同步的交互轮次。\n[3/3] 同步完成。\n")
		return nil
	}

	fmt.Fprintf(out, "[2/3] 正在向服务端上传转录事件 (目标: %s)...\n", syncEndpoint)

	const chunkSize = 200
	totalInserted := 0
	totalSkipped := 0
	client := &http.Client{Timeout: 60 * time.Second}

	for i := 0; i < len(summary.Events); i += chunkSize {
		end := i + chunkSize
		if end > len(summary.Events) {
			end = len(summary.Events)
		}

		batch := storage.SyncBatch{
			NodeID: n.NodeID,
			Events: summary.Events[i:end],
		}

		bodyBytes, err := json.Marshal(batch)
		if err != nil {
			return fmt.Errorf("序列化批次失败: %w", err)
		}

		var lastErr error
		var batchInserted, batchSkipped int
		maxRetries := 3

		for attempt := 1; attempt <= maxRetries; attempt++ {
			req, err := http.NewRequest(http.MethodPost, syncEndpoint, bytes.NewReader(bodyBytes))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			if cfg.Report.APIKey != "" {
				req.Header.Set("X-API-Key", cfg.Report.APIKey)
			}

			resp, err := client.Do(req)
			if err != nil {
				lastErr = err
				if attempt < maxRetries {
					time.Sleep(time.Duration(attempt) * time.Second)
					continue
				}
				return fmt.Errorf("推送批次失败: %w", lastErr)
			}

			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return fmt.Errorf("服务端同步接口不可用 (HTTP 404: %s)\n提示: 远端服务尚未启用批量同步接口，请在服务器上更新并重启 statusline-server 服务", syncEndpoint)
			}

			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode >= 500 && attempt < maxRetries {
					time.Sleep(time.Duration(attempt) * time.Second)
					continue
				}
				return fmt.Errorf("服务端返回异常 (HTTP %d): %s", resp.StatusCode, string(respBody))
			}

			var res struct {
				Status   string `json:"status"`
				Received int    `json:"received"`
				Inserted int    `json:"inserted"`
				Skipped  int    `json:"skipped"`
			}
			if err := json.Unmarshal(respBody, &res); err != nil {
				return fmt.Errorf("解析服务端响应失败: %w", err)
			}

			batchInserted = res.Inserted
			batchSkipped = res.Skipped
			lastErr = nil
			break
		}

		if lastErr != nil {
			return fmt.Errorf("推送批次重试失败: %w", lastErr)
		}

		totalInserted += batchInserted
		totalSkipped += batchSkipped
		fmt.Fprintf(out, "      已处理: %d/%d (新增入库: %d, 跳过重复: %d)...\n", end, len(summary.Events), totalInserted, totalSkipped)

		// Brief pause between chunks to allow SQLite WAL checkpoint and concurrent requests
		time.Sleep(20 * time.Millisecond)
	}

	fmt.Fprintf(out, "[3/3] 转录同步完成!\n")
	fmt.Fprintf(out, "      ✓ 本地扫描: %d 个文件，%d 条轮次\n", summary.FilesScanned, summary.TotalEvents)
	fmt.Fprintf(out, "      ✓ 服务端新增入库: %d 条\n", totalInserted)
	fmt.Fprintf(out, "      ✓ 服务端跳过重复: %d 条\n", totalSkipped)

	return nil
}

func execute(r io.Reader, w io.Writer, cfg *config.Config, n node.NodeInfo) {
	if cfg != nil {
		render.SetAliases(cfg.Aliases)
	}
	raw, _ := io.ReadAll(r)
	slog.Debug("stdin read", "bytes", len(raw))

	env, data, _ := model.ParseAndNormalize(raw, n)

	// IMP-1: Probe Git status if cwd or workspace dir is present
	targetDir := data.Workspace.CurrentDir
	if targetDir == "" {
		targetDir = data.CWD
	}
	if targetDir != "" {
		branch, dirty, ahead, behind := vcs.GetGitStatus(targetDir, 20*time.Millisecond)
		if branch != "" {
			if data.VCS.Branch == "" {
				data.VCS.Branch = branch
			}
			data.VCS.Dirty = data.VCS.Dirty || dirty
			data.VCS.Ahead = ahead
			data.VCS.Behind = behind
			if env != nil {
				env.Normalized.GitBranch = data.VCS.Branch
			}
			slog.Debug("git status probed", "branch", branch, "dirty", dirty, "ahead", ahead, "behind", behind)
		}
	}

	homeDir, _ := os.UserHomeDir()
	var segments []string
	if cfg != nil && len(cfg.Display.Segments) > 0 {
		segments = cfg.Display.Segments
	}
	var priorities []string
	if cfg != nil && len(cfg.Display.Priorities) > 0 {
		priorities = cfg.Display.Priorities
	}
	line := render.AssembleLine(data, homeDir, segments, priorities)
	_, _ = io.WriteString(w, line)

	// Probe real API tokens from transcript tail if path is available
	if env != nil && env.Normalized.TranscriptPath != "" {
		usage := transcript.ProbeRecentUsage(env.Normalized.TranscriptPath, env.Normalized.SessionID, 5*time.Millisecond)
		if usage.PromptID != "" {
			env.Normalized.PromptID = usage.PromptID
		}
		if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheReadTokens > 0 || usage.CacheCreationTokens > 0 {
			env.Normalized.CurrentInputTokens = usage.InputTokens
			env.Normalized.CurrentOutputTokens = usage.OutputTokens
			env.Normalized.CacheReadTokens = usage.CacheReadTokens
			env.Normalized.CacheCreationTokens = usage.CacheCreationTokens
		}
	}

	// CRIT-1: Terminal output is already written. Synchronously report with strict timeout
	// so Go runtime doesn't terminate before HTTP packets leave the machine.
	if cfg != nil && cfg.Report.Endpoint != "" && cfg.Report.Endpoint != "off" && cfg.Report.Endpoint != "none" {
		timeout := time.Duration(cfg.Report.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		reporter.Report(env, cfg.Report.Endpoint, cfg.Report.APIKey, timeout)
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			if err := installer.RunInit(os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing statusline: %v\n", err)
				os.Exit(1)
			}
			return
		case "sync":
			if err := runSyncCommand(os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error synchronizing transcripts: %v\n", err)
				os.Exit(1)
			}
			return
		case "upgrade", "update":
			if err := runUpgradeCommand(os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading statusline: %v\n", err)
				os.Exit(1)
			}
			return
		case "auto-update":
			cfg, err := config.LoadConfig("")
			if err != nil {
				cfg = config.DefaultConfig()
			}
			var out io.Writer = os.Stdout
			// If not running in terminal (e.g. background detached process), discard stdout
			if fi, err := os.Stdout.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
				out = nil
			}
			cleanup := logging.Setup(true, "info", "text", "statusline.log")
			defer cleanup()
			if err := runAutoUpdateCommand(os.Args[2:], cfg, out); err != nil {
				if out != nil {
					fmt.Fprintf(os.Stderr, "Error in auto-update: %v\n", err)
				}
				os.Exit(1)
			}
			return
		case "--version", "-v", "-version", "version":
			fmt.Printf("statusline version %s\n", Version)
			return
		case "--help", "-h", "-help", "help":
			printHelp()
			return
		}
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Initialize structured logging (disabled by default for client)
	cleanup := logging.Setup(cfg.Logging.Enabled, cfg.Logging.Level, cfg.Logging.Format, "statusline.log")
	defer cleanup()

	slog.Debug("config loaded", "endpoint", cfg.Report.Endpoint, "segments", cfg.Display.Segments)

	n := node.GetNodeInfo(cfg.Node.ID, Version)
	slog.Debug("node info", "node_id", n.NodeID, "hostname", n.Hostname)
	execute(os.Stdin, os.Stdout, cfg, n)

	// Trigger daily background sync & upgrade check after render completes
	autoupdate.TriggerDailySyncIfDue(cfg, Version)
}
