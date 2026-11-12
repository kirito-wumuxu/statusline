package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"statusline/internal/config"
	"statusline/internal/logging"
	"statusline/internal/server"
	"statusline/internal/storage"
)

//go:embed web/*
var embeddedWebFS embed.FS

var webFS fs.FS

func init() {
	sub, err := fs.Sub(embeddedWebFS, "web")
	if err == nil {
		webFS = sub
	}
}

func main() {
	portFlag := flag.Int("port", 0, "HTTP server listening port (default: 8628)")
	dbFlag := flag.String("db", "", "Path to SQLite database file")
	configFlag := flag.String("config", "", "Path to server.yaml")
	flag.Parse()

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
