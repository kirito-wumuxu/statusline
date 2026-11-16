package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"statusline/internal/config"
	"statusline/internal/logging"
	"statusline/internal/server"
	"statusline/internal/storage"
	"statusline/internal/updater"
)

//go:embed web/*
var embeddedWebFS embed.FS

var webFS fs.FS

var Version = "dev"

func init() {
	sub, err := fs.Sub(embeddedWebFS, "web")
	if err == nil {
		webFS = sub
	}
}

func printHelp() {
	displayVer := Version
	if !strings.HasPrefix(displayVer, "v") && !strings.HasPrefix(displayVer, "V") && displayVer != "dev" {
		displayVer = "v" + displayVer
	}
	fmt.Printf(`statusline-server %s - Centralized telemetry server & real-time web dashboard

Usage:
  statusline-server [options]          Start the telemetry HTTP server & Web dashboard
  statusline-server upgrade [options]  Self-upgrade statusline-server to the latest release
  statusline-server update [options]   Alias for upgrade
  statusline-server --version, -v      Print version information
  statusline-server --help, -h         Show this help message

Server Options:
  -port <port>                         HTTP server listening port (default: 8628)
  -db <path>                           Path to SQLite database file
  -config <path>                       Path to server.yaml

Upgrade Options:
  -check                               Check for new version without downloading or installing
  -all                                 Also upgrade statusline client if present in same directory
  -version <tag>                       Target version tag to install (default: latest)
  -source <auto|server|github>         Update source: auto, server, or github (default: auto)
  -y, --yes                            Skip confirmation prompts
`, displayVer)
}

func runUpgradeCommand(args []string, out io.Writer) error {
	return updater.RunUpgrade(args, Version, out)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "upgrade", "update":
			if err := runUpgradeCommand(os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading statusline-server: %v\n", err)
				os.Exit(1)
			}
			return
		case "--help", "-h", "-help", "help":
			printHelp()
			return
		case "--version", "-v", "-version", "version":
			fmt.Printf("statusline-server version %s\n", Version)
			return
		}
	}
	portFlag := flag.Int("port", 0, "HTTP server listening port (default: 8628)")
	dbFlag := flag.String("db", "", "Path to SQLite database file")
	configFlag := flag.String("config", "", "Path to server.yaml")
	versionFlag := flag.Bool("version", false, "Print version information")
	flag.BoolVar(versionFlag, "v", false, "Print version information")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("statusline-server version %s\n", Version)
		return
	}

	cfg, err := config.LoadServerConfig(*configFlag)
	if err != nil {
		log.Printf("Warning: failed to load server config: %v, using defaults", err)
		cfg = config.DefaultServerConfig()
	}

	// Initialize structured logging (enabled by default for server)
	cleanup := logging.Setup(cfg.Logging.Enabled, cfg.Logging.Level, cfg.Logging.Format, "statusline-server.log")
	defer cleanup()

	if *portFlag > 0 {
		cfg.Server.Port = *portFlag
	}
	if *dbFlag != "" {
		cfg.Storage.DBPath = *dbFlag
	}

	store, err := storage.NewStore(cfg.Storage.DBPath)
	if err != nil {
		slog.Error("failed to initialize SQLite database", "db_path", cfg.Storage.DBPath, "error", err)
		log.Fatalf("Fatal: failed to initialize SQLite database at %s: %v", cfg.Storage.DBPath, err)
	}
	defer store.Close()

	srv := server.NewServer(cfg, store, webFS)
	srv.SetVersion(Version)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.Host == "0.0.0.0" || cfg.Server.Host == "" {
		addr = fmt.Sprintf(":%d", cfg.Server.Port)
	}
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("StatusLine Server starting", "port", cfg.Server.Port, "db_path", cfg.Storage.DBPath)
		log.Printf("StatusLine Server listening on http://127.0.0.1:%d (DB: %s)", cfg.Server.Port, cfg.Storage.DBPath)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-stopCh
	slog.Info("shutting down StatusLine Server")
	log.Println("Shutting down StatusLine Server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		log.Printf("Server shutdown error: %v", err)
	}
	slog.Info("server gracefully stopped")
	log.Println("Server gracefully stopped")
}
