package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"statusline/internal/config"
	"statusline/internal/logging"
	"statusline/internal/session"
	"statusline/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
)

var Version = "dev"

func printHelp() {
	displayVer := Version
	if !strings.HasPrefix(displayVer, "v") && !strings.HasPrefix(displayVer, "V") && displayVer != "dev" {
		displayVer = "v" + displayVer
	}
	fmt.Printf(`statusline-session %s (alias: sls)
Terminal session manager & fast switcher for AI CLIs (Google Antigravity & Claude Code)

Usage:
  statusline-session [options]

Options:
  -all-nodes, -a     Show sessions from all devices (default: local device only)
  -source <mode>     Session data source: auto (default), remote (API only), db (SQLite only)
  -db <path>         Custom path to statusline SQLite database
  -config <path>     Custom path to client.yaml or server.yaml
  -version, -v       Print version information
  -help, -h          Show this help message

Keybindings in TUI:
  [↑/k] 上移选择       [↓/j] 下移选择       [/] 即时搜索过滤
  [Enter] 恢复接管此会话                    [t] 在 Tmux 新窗口打开
  [c] 复制恢复命令至剪贴板                 [a] 切换本设备/全设备过滤
  [r] 刷新会话列表                         [q/Esc] 退出程序
`, displayVer)
}

func main() {
	allNodesFlag := flag.Bool("all-nodes", false, "Show sessions from all nodes")
	flag.BoolVar(allNodesFlag, "a", false, "Alias for -all-nodes")
	sourceFlag := flag.String("source", "auto", "Data source: auto (prefer remote API with local fallback), remote (API only), db (SQLite only)")
	dbPathFlag := flag.String("db", "", "Path to SQLite database")
	configPathFlag := flag.String("config", "", "Path to client.yaml or server.yaml")
	versionFlag := flag.Bool("version", false, "Print version")
	flag.BoolVar(versionFlag, "v", false, "Alias for -version")
	helpFlag := flag.Bool("help", false, "Show help message")
	flag.BoolVar(helpFlag, "h", false, "Alias for -help")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("statusline-session version %s\n", Version)
		return
	}
	if *helpFlag {
		printHelp()
		return
	}

	// 1. Load configuration
	cfg, err := config.LoadClientConfig(*configPathFlag)
	if err != nil {
		cfg = config.DefaultClientConfig()
	}

	// 2. Initialize logging (default: nopHandler prevents terminal stderr pollution)
	cleanup := logging.Setup(cfg.Logging.Enabled, cfg.Logging.Level, cfg.Logging.Format, "statusline-session.log")
	defer cleanup()

	// If user explicitly passed a custom -db path, set source to db unless overridden
	if *dbPathFlag != "" && *sourceFlag == "auto" {
		*sourceFlag = "db"
	}

	// 3. Open SQLite database if not in pure remote mode
	var store *storage.Store
	if *sourceFlag != "remote" {
		dbPath := *dbPathFlag
		if dbPath == "" {
			homeDir, _ := os.UserHomeDir()
			if homeDir != "" {
				dbPath = filepath.Join(homeDir, ".local", "share", "statusline", "statusline.db")
			}
		}
		if dbPath != "" {
			if fi, err := os.Stat(dbPath); err == nil && !fi.IsDir() {
				if s, err := storage.NewStore(dbPath); err == nil {
					defer s.Close()
					store = s
				}
			}
		}
	}

	// 4. Initialize session service
	svc := session.NewSessionService(store, cfg)
	svc.SetSourceMode(session.SourceMode(*sourceFlag))

	// 5. Run Bubble Tea TUI
	initialOnlyLocal := !*allNodesFlag
	app := NewAppModel(svc, initialOnlyLocal)

	p := tea.NewProgram(app, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "运行 TUI 失败: %v\n", err)
		os.Exit(1)
	}

	// 5. Handle post-TUI launch action
	if m, ok := finalModel.(*AppModel); ok && m.launchAction != nil {
		targetItem := *m.launchAction
		cmdName, cmdArgs, cwd, _ := session.BuildResumeCommand(targetItem)

		if !session.CheckWorkingDir(cwd) {
			fmt.Fprintf(os.Stderr, "❌ 无法启动：工作目录 %q 在本机不存在或已被移除。\n", cwd)
			os.Exit(1)
		}

		fmt.Printf("🚀 正在切入目录 %s 并恢复会话...\n", cwd)
		c := exec.Command(cmdName, cmdArgs...)
		c.Dir = cwd
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "会话执行结束: %v\n", err)
		}
	}
}
