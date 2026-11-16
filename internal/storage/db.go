package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// OpenDB opens a connection to the SQLite database and executes migrations and pragmas.
func OpenDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("empty database path")
	}

	// Create parent directory if writing to file
	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory %s: %w", dir, err)
		}
	}

	dsn := dbPath
	if !strings.Contains(dsn, "?") {
		dsn = dsn + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Set connection pool limits
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)

	// Execute migrations
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to execute schema migration: %w", err)
	}

	// Idempotent column migrations for existing databases
	_, _ = db.Exec("ALTER TABLE display_aliases ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;")
	_, _ = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_events_session_prompt ON telemetry_events(session_id, prompt_id) WHERE prompt_id IS NOT NULL AND prompt_id != '';")

	// Initialize default display aliases
	if err := initDefaultAliases(db); err != nil {
		slog.Warn("failed to initialize default display aliases", "error", err)
	}

	slog.Info("sqlite database opened", "path", dbPath, "max_open_conns", 50)

	return db, nil
}

func initDefaultAliases(db *sql.DB) error {
	defaultRecords := []struct {
		category  string
		key       string
		name      string
		short     string
		sortOrder int
	}{
		// Quotas (去时间后缀的前缀)
		{"quota", "gemini", "Gemini", "G", 0},
		{"quota", "gemini-argon", "Argon", "Ar", 0},
		{"quota", "3p", "Claude", "C", 0},
		{"quota", "claude", "Claude", "C", 0},
		// Models
		{"model", "gemini-3.8-flash", "Gemini 3.8 Flash", "Flash", 0},
		{"model", "gemini-3.8-pro", "Gemini 3.8 Pro", "Pro", 0},
		{"model", "claude-3-7-sonnet", "Claude 3.7 Sonnet", "Sonnet", 0},
		{"model", "claude-opus", "Claude Opus", "Opus", 0},
		// Priorities
		{"priority", "model", "模型标识与推理深度", "P1", 1},
		{"priority", "context", "上下文进度条与百分比", "P2", 2},
		{"priority", "quota", "供应商配额余量与倒计时", "P3", 3},
		{"priority", "vcs", "Git 仓库分支与脏标记", "P4", 4},
		{"priority", "tokens", "会话 Token 吞吐计数器", "P5", 5},
		{"priority", "account", "登录账号标识与邮箱", "P6", 6},
		{"priority", "dir", "工作区目录", "P7", 7},
	}

	stmt, err := db.Prepare(`
		INSERT OR IGNORE INTO display_aliases (category, alias_key, display_name, short_name, sort_order, updated_at)
		VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range defaultRecords {
		if _, err := stmt.Exec(r.category, r.key, r.name, r.short, r.sortOrder); err != nil {
			return err
		}
	}
	return nil
}
