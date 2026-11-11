package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"statusline/internal/config"
	"statusline/internal/installer"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/reporter"
	"statusline/internal/vcs"
)

var Version = "1.0.0"

func printHelp() {
	fmt.Printf(`statusline v%s - Terminal statusline plugin for Antigravity & Claude Code

Usage:
  statusline                    Render statusline from stdin JSON (called by AI CLI)
  statusline init [options]     Automatically configure Antigravity settings.json
  statusline --version, -v      Print version information
  statusline --help, -h         Show this help message

Init Options:
  -settings <path>              Specify custom path to settings.json
  -bin <path>                   Specify custom binary path to configure
  -no-copy                      Skip copying binary to ~/.local/bin
`, Version)
}

func execute(r io.Reader, w io.Writer, cfg *config.Config, n node.NodeInfo) {
	raw, _ := io.ReadAll(r)

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
		}
	}

	homeDir, _ := os.UserHomeDir()
	var segments []string
	if cfg != nil && len(cfg.Display.Segments) > 0 {
		segments = cfg.Display.Segments
	}
	line := render.AssembleLine(data, homeDir, segments)
	_, _ = io.WriteString(w, line)

	// CRIT-1: Terminal output is already written. Synchronously report with strict timeout
	// so Go runtime doesn't terminate before HTTP packets leave the machine.
	if cfg != nil && cfg.Server.Endpoint != "" && cfg.Server.Endpoint != "off" && cfg.Server.Endpoint != "none" {
		timeout := time.Duration(cfg.Server.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		reporter.Report(env, cfg.Server.Endpoint, timeout)
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
		case "--version", "-v", "version":
			fmt.Printf("statusline version %s\n", Version)
			return
		case "--help", "-h", "help":
			printHelp()
			return
		}
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		cfg = config.DefaultConfig()
	}

	n := node.GetNodeInfo(cfg.Node.ID, Version)
	execute(os.Stdin, os.Stdout, cfg, n)
}
