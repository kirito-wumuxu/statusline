package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"statusline/internal/config"
	"statusline/internal/installer"
	"statusline/internal/logging"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/reporter"
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
  statusline init [options]     Auto-configure settings.json for Antigravity & Claude Code
  statusline upgrade [options]  Self-upgrade statusline to the latest release
  statusline update [options]   Alias for upgrade
  statusline --version, -v      Print version information
  statusline --help, -h         Show this help message

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

func runUpgradeCommand(args []string, out io.Writer) error {
	return updater.RunUpgrade(args, Version, out)
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
		case "upgrade", "update":
			if err := runUpgradeCommand(os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading statusline: %v\n", err)
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
}
