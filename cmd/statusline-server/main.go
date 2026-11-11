package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"statusline/internal/config"
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
	portFlag := flag.Int("port", 0, "HTTP server listening port")
	dbFlag := flag.String("db", "", "Path to SQLite database file")
	configFlag := flag.String("config", "", "Path to server.yaml (or config.yaml)")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*configFlag)
	if err != nil {
		log.Printf("Warning: failed to load server config: %v, using defaults", err)
		cfg = config.DefaultServerConfig()
	}

	if *portFlag > 0 {
		cfg.Server.Port = *portFlag
	}
	if *dbFlag != "" {
		cfg.Storage.DBPath = *dbFlag
		cfg.Server.DBPath = *dbFlag
	}

	store, err := storage.NewStore(cfg.Storage.DBPath)
	if err != nil {
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
		log.Printf("StatusLine Server listening on http://127.0.0.1:%d (DB: %s)", cfg.Server.Port, cfg.Storage.DBPath)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-stopCh
	log.Println("Shutting down StatusLine Server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server gracefully stopped")
}
