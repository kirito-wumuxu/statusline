package main

import (
	"io"
	"os"
	"time"

	"statusline/internal/config"
	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/render"
	"statusline/internal/reporter"
)

var Version = "1.0.0"

func execute(r io.Reader, w io.Writer, cfg *config.Config, n node.NodeInfo) {
	raw, _ := io.ReadAll(r)

	env, data, _ := model.ParseAndNormalize(raw, n)

	homeDir, _ := os.UserHomeDir()
	line := render.AssembleLine(data, homeDir)
	_, _ = io.WriteString(w, line)

	if cfg != nil && cfg.Server.Endpoint != "" && cfg.Server.Endpoint != "off" && cfg.Server.Endpoint != "none" {
		timeout := time.Duration(cfg.Server.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 100 * time.Millisecond
		}
		reporter.ReportAsync(env, cfg.Server.Endpoint, timeout)
	}
}

func main() {
	cfg, err := config.LoadConfig("")
	if err != nil {
		cfg = config.DefaultConfig()
	}

	n := node.GetNodeInfo(cfg.Node.ID, Version)
	execute(os.Stdin, os.Stdout, cfg, n)
}
