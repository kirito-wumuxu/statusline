package autoupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"statusline/internal/config"
	"statusline/internal/installer"
	"statusline/internal/updater"
)

// SyncState persists the daily synchronization and update status.
type SyncState struct {
	LastDate     string `json:"last_date"`               // YYYY-MM-DD
	LastTime     string `json:"last_time"`               // RFC3339 timestamp
	LastStatus   string `json:"last_status"`             // "pending", "success", "error"
	UpgradedTo   string `json:"upgraded_to,omitempty"`   // Installed version if an upgrade happened
	ErrorMessage string `json:"error_message,omitempty"` // Failure message if any
}

// GetStateFilePath returns the path to the daily sync state marker.
func GetStateFilePath() string {
	if env := os.Getenv("STATUSLINE_STATE_FILE"); env != "" {
		return env
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		dir := filepath.Join(homeDir, ".local", "share", "statusline")
		_ = os.MkdirAll(dir, 0755)
		return filepath.Join(dir, ".daily_sync")
	}
	return ".daily_sync"
}

// IsDailySyncDue returns true if no sync has been recorded for the given date.
func IsDailySyncDue(statePath string, now time.Time) bool {
	today := now.Format("2006-01-02")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return true
	}
	var st SyncState
	if err := json.Unmarshal(data, &st); err != nil {
		return strings.TrimSpace(string(data)) != today
	}
	return st.LastDate != today
}

// MarkDailySyncTriggered atomically marks the sync state as pending for today to prevent duplicates.
func MarkDailySyncTriggered(statePath string, now time.Time) error {
	today := now.Format("2006-01-02")
	st := SyncState{
		LastDate:   today,
		LastTime:   now.Format(time.RFC3339),
		LastStatus: "pending",
	}
	bytes, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(statePath), 0755)
	return os.WriteFile(statePath, bytes, 0644)
}

// MarkDailySyncCompleted records the final completion outcome of the daily sync/upgrade.
func MarkDailySyncCompleted(statePath string, now time.Time, upgradedTo string, err error) {
	today := now.Format("2006-01-02")
	st := SyncState{
		LastDate: today,
		LastTime: now.Format(time.RFC3339),
	}
	if err != nil {
		st.LastStatus = "error"
		st.ErrorMessage = err.Error()
	} else {
		st.LastStatus = "success"
		st.UpgradedTo = upgradedTo
	}
	bytes, _ := json.MarshalIndent(st, "", "  ")
	_ = os.MkdirAll(filepath.Dir(statePath), 0755)
	_ = os.WriteFile(statePath, bytes, 0644)
}

// TriggerDailySyncIfDue inspects if today is the first invocation of the day.
// If due, it instantly marks the state and spawns a detached background process to perform sync & upgrade.
func TriggerDailySyncIfDue(cfg *config.Config, currentVer string) {
	if cfg == nil || !cfg.AutoSync.Enabled {
		return
	}
	statePath := GetStateFilePath()
	now := time.Now()
	if !IsDailySyncDue(statePath, now) {
		return
	}

	// Immediately mark as triggered to eliminate race conditions from rapid successive invocations
	if err := MarkDailySyncTriggered(statePath, now); err != nil {
		slog.Warn("failed to mark daily sync triggered", "error", err)
	}

	// Spawn detached background worker
	if err := SpawnDetachedAutoUpdate(); err != nil {
		slog.Warn("failed to spawn detached auto-update", "error", err)
	} else {
		slog.Debug("spawned detached auto-update process")
	}
}

// RunAutoUpdate executes the full auto-update workflow:
// 1. Sync display aliases and module priorities from statusline-server into client.yaml.
// 2. Check for newer release versions and perform in-place atomic upgrade.
func RunAutoUpdate(cfg *config.Config, currentVer string, force, check bool, out io.Writer) error {
	statePath := GetStateFilePath()
	now := time.Now()

	if !force && !check && !IsDailySyncDue(statePath, now) {
		if out != nil {
			fmt.Fprintf(out, "ℹ 今日已完成自动检查与配置同步，无需重复执行（可用 -force 强制执行）。\n")
		}
		return nil
	}

	_ = MarkDailySyncTriggered(statePath, now)

	slog.Info("auto-update workflow started", "current_version", currentVer, "force", force, "check", check)

	// Step 1: Sync aliases and display priorities
	if cfg == nil || cfg.AutoSync.SyncAliases {
		if out != nil {
			fmt.Fprintf(out, "[1/2] 正在从服务端同步显示别名与模块优先级...\n")
		}
		if check {
			if out != nil {
				fmt.Fprintf(out, "      [check] 将连接 %s 并更新本地 client.yaml\n", cfg.Report.Endpoint)
			}
		} else {
			var buf bytes.Buffer
			installer.SyncAliasesFromRemote(&buf)
			outputStr := strings.TrimSpace(buf.String())
			slog.Info("sync aliases completed", "details", outputStr)
			if out != nil && len(outputStr) > 0 {
				fmt.Fprintf(out, "%s\n", outputStr)
			}
		}
	} else if out != nil {
		fmt.Fprintf(out, "[1/2] 显示别名与优先级自动同步已在配置中停用，跳过。\n")
	}

	// Step 2: Auto-upgrade binary if newer version available
	var upgradedVer string
	var upgradeErr error

	if cfg == nil || cfg.AutoSync.AutoUpgrade {
		if out != nil {
			fmt.Fprintf(out, "[2/2] 正在检查可用新版本...\n")
		}

		src, err := updater.ResolveSource(cfg, "auto")
		if err != nil {
			slog.Warn("failed to resolve update source", "error", err)
			upgradeErr = err
			if out != nil {
				fmt.Fprintf(out, "⚠ 探测更新源失败: %v\n", err)
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			rel, err := src.CheckRelease(ctx, "latest")
			if err != nil {
				slog.Warn("check release failed", "source", src.Name(), "error", err)
				upgradeErr = err
				if out != nil {
					fmt.Fprintf(out, "⚠ 检查新版本失败 (%s): %v\n", src.Name(), err)
				}
			} else if rel != nil && updater.IsNewer(rel.Version, currentVer) {
				slog.Info("newer release found", "current", currentVer, "target", rel.Version)
				if currentVer == "dev" {
					slog.Info("current binary is development build (dev), skipping auto-upgrade to protect local binary")
					if out != nil {
						fmt.Fprintf(out, "ℹ 发现新版本 %s，但当前运行的是开发构建版本 (dev)，为保护本地源码环境已跳过自动升级。\n", rel.Version)
					}
				} else if check {
					if out != nil {
						fmt.Fprintf(out, "✓ 发现新版本: %s -> %s (check 模式，未执行下载与替换)\n", currentVer, rel.Version)
					}
				} else {
					if out != nil {
						fmt.Fprintf(out, "发现新版本 %s -> %s，正在执行原地自动升级...\n", currentVer, rel.Version)
					}
					var upBuf bytes.Buffer
					var targetWriter io.Writer = &upBuf
					if out != nil {
						targetWriter = io.MultiWriter(&upBuf, out)
					}

					execPath, _ := updater.GetExecutablePath()
					uOpts := updater.UpgradeOptions{
						TargetTag:        rel.Version,
						ForcedSource:     "auto",
						UpgradeAll:       true,
						Yes:              true,
						CurrentVer:       currentVer,
						TargetExecutable: execPath,
						ClientConfig:     cfg,
					}
					err = updater.ExecuteUpgrade(uOpts, targetWriter)
					if err != nil {
						slog.Error("auto-upgrade failed", "error", err, "log", upBuf.String())
						upgradeErr = err
						if out != nil {
							fmt.Fprintf(out, "✗ 自动升级失败: %v\n", err)
						}
					} else {
						upgradedVer = rel.Version
						slog.Info("auto-upgrade succeeded", "version", rel.Version)
						if out != nil {
							fmt.Fprintf(out, "✓ 自动升级至 %s 完成！\n", rel.Version)
						}
					}
				}
			} else {
				slog.Info("statusline is already up to date", "version", currentVer)
				if out != nil {
					fmt.Fprintf(out, "✓ 当前已是最新版本 (%s)，无需升级。\n", currentVer)
				}
			}
		}
	} else if out != nil {
		fmt.Fprintf(out, "[2/2] 自动版本升级已在配置中停用，跳过。\n")
	}

	MarkDailySyncCompleted(statePath, now, upgradedVer, upgradeErr)
	return upgradeErr
}
