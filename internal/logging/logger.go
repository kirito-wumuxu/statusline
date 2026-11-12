package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Setup initializes the global slog.Default() logger.
// Log file is always created in the same directory as the running executable.
// Returns a cleanup function to close the log file handle.
func Setup(enabled bool, level, format, logFileName string) func() {
	if !enabled {
		slog.SetDefault(slog.New(nopHandler{}))
		return func() {}
	}

	// Resolve executable directory for log file placement
	exeDir := resolveExeDir()
	logPath := filepath.Join(exeDir, logFileName)

	// Parse log level
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Open log file (append mode, create if not exists)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// Cannot open log file — fall back to disabled
		slog.SetDefault(slog.New(nopHandler{}))
		return func() {}
	}

	opts := &slog.HandlerOptions{Level: slogLevel}
	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(f, opts)
	} else {
		handler = slog.NewTextHandler(f, opts)
	}

	slog.SetDefault(slog.New(handler))
	return func() { _ = f.Close() }
}

// resolveExeDir returns the directory containing the current executable, resolving symlinks.
var resolveExeDir = defaultResolveExeDir

func defaultResolveExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// nopHandler discards all log records. slog.Enabled() returns false for all levels,
// guaranteeing zero allocation when logging is disabled.
type nopHandler struct{}

func (nopHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (nopHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h nopHandler) WithAttrs(_ []slog.Attr) slog.Handler        { return h }
func (h nopHandler) WithGroup(_ string) slog.Handler              { return h }
