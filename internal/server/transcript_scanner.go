package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"statusline/internal/storage"
)

// SyncResult captures the summary metrics of a transcript synchronization pass.
type SyncResult struct {
	ScannedFiles int   `json:"scanned_files"`
	NewEvents    int   `json:"new_events"`
	ElapsedMS    int64 `json:"elapsed_ms"`
}

// TranscriptScanner scans local conversation transcript logs and synchronizes missing turn events into SQLite.
type TranscriptScanner struct {
	store   *storage.Store
	homeDir string
}

// NewTranscriptScanner initializes a new scanner with the given storage engine.
func NewTranscriptScanner(store *storage.Store) *TranscriptScanner {
	h, _ := os.UserHomeDir()
	return &TranscriptScanner{store: store, homeDir: h}
}

// SetHomeDir overrides the home directory for discovery (useful in tests).
func (s *TranscriptScanner) SetHomeDir(dir string) {
	s.homeDir = dir
}

type sessionCandidate struct {
	SessionID      string
	NodeID         string
	ProductType    string
	LatestModelID  string
	TranscriptPath string
}

type transcriptStep struct {
	StepIndex           *int64 `json:"step_index,omitempty"`
	Type                string `json:"type,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
	Timestamp           string `json:"timestamp,omitempty"`
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
	CacheReadTokens     *int64 `json:"cache_read_tokens,omitempty"`
	UUID                string `json:"uuid,omitempty"`
	MessageID           string `json:"message_id,omitempty"`

	Message *struct {
		Model string `json:"model,omitempty"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`
}

// SyncAll discovers and processes all transcripts, idempotently backfilling missing events.
func (s *TranscriptScanner) SyncAll(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	res := &SyncResult{}

	candidates, err := s.discoverTranscripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover transcripts: %w", err)
	}

	for _, cand := range candidates {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		newCount, err := s.syncSingleFile(ctx, cand)
		if err == nil {
			res.ScannedFiles++
			res.NewEvents += newCount
		}
	}

	res.ElapsedMS = time.Since(start).Milliseconds()
	return res, nil
}

func (s *TranscriptScanner) discoverTranscripts(ctx context.Context) ([]sessionCandidate, error) {
	candMap := make(map[string]sessionCandidate) // key: transcript_path

	// 1. Discover from database `sessions`
	rows, err := s.store.DB().QueryContext(ctx, "SELECT session_id, node_id, product_type, COALESCE(latest_model_id, ''), transcript_path FROM sessions WHERE transcript_path IS NOT NULL AND transcript_path != ''")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c sessionCandidate
			if err := rows.Scan(&c.SessionID, &c.NodeID, &c.ProductType, &c.LatestModelID, &c.TranscriptPath); err == nil {
				candMap[c.TranscriptPath] = c
			}
		}
	}

	// 2. Discover from standard Antigravity directories: ~/.gemini/antigravity*/brain/*/.system_generated/logs/transcript.jsonl
	homeDir := s.homeDir
	if homeDir != "" {
		agyPatterns := []string{
			filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", "*", ".system_generated", "logs", "transcript.jsonl"),
			filepath.Join(homeDir, ".gemini", "antigravity", "brain", "*", ".system_generated", "logs", "transcript.jsonl"),
			filepath.Join(homeDir, ".gemini", "antigravity-ide", "brain", "*", ".system_generated", "logs", "transcript.jsonl"),
		}
		for _, pat := range agyPatterns {
			matches, _ := filepath.Glob(pat)
			for _, m := range matches {
				if _, exists := candMap[m]; !exists {
					// Extract session ID from directory structure: .../brain/<session-id>/.system_generated/...
					parts := strings.Split(filepath.ToSlash(m), "/")
					sessID := ""
					for i, part := range parts {
						if part == ".system_generated" && i > 0 {
							sessID = parts[i-1]
							break
						}
					}
					if sessID == "" {
						sessID = filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(m))))
					}
					candMap[m] = sessionCandidate{
						SessionID:      sessID,
						NodeID:         "local_discovered",
						ProductType:    "antigravity",
						LatestModelID:  "gemini-3.8-flash-high",
						TranscriptPath: m,
					}
				}
			}
		}

		// 3. Discover from standard Claude Code directories: ~/.claude/projects/*/*.jsonl
		claudeMatches, _ := filepath.Glob(filepath.Join(homeDir, ".claude", "projects", "*", "*.jsonl"))
		for _, m := range claudeMatches {
			if _, exists := candMap[m]; !exists {
				sessID := strings.TrimSuffix(filepath.Base(m), ".jsonl")
				candMap[m] = sessionCandidate{
					SessionID:      sessID,
					NodeID:         "local_discovered",
					ProductType:    "claude_code",
					LatestModelID:  "claude-3-7-sonnet",
					TranscriptPath: m,
				}
			}
		}
	}

	var list []sessionCandidate
	for _, c := range candMap {
		list = append(list, c)
	}
	return list, nil
}

type eventToInsert struct {
	SessionID           string
	NodeID              string
	PromptID            string
	ProductType         string
	ReportedAt          string
	ModelID             string
	ModelDisplayName    string
	CurrentInputTokens  int64
	CurrentOutputTokens int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

func (s *TranscriptScanner) syncSingleFile(ctx context.Context, cand sessionCandidate) (int, error) {
	file, err := os.Open(cand.TranscriptPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// Retrieve existing prompt_ids for this session
	existingPrompts := make(map[string]bool)
	rows, err := s.store.DB().QueryContext(ctx, "SELECT prompt_id FROM telemetry_events WHERE session_id = ? AND prompt_id IS NOT NULL", cand.SessionID)
	if err == nil {
		for rows.Next() {
			var pid string
			if err := rows.Scan(&pid); err == nil && pid != "" {
				existingPrompts[pid] = true
			}
		}
		rows.Close()
	}

	var toInsert []eventToInsert
	reader := bufio.NewReader(file)
	lineIdx := 0

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) > 0 {
			lineIdx++
			trimmed := bytes.TrimSpace(lineBytes)
			if len(trimmed) > 0 {
				var step transcriptStep
				if jsonErr := json.Unmarshal(trimmed, &step); jsonErr == nil {
					promptID := ""
					if step.StepIndex != nil {
						promptID = fmt.Sprintf("step_%d", *step.StepIndex)
					} else if step.UUID != "" {
						promptID = step.UUID
					} else if step.MessageID != "" {
						promptID = step.MessageID
					} else {
						promptID = fmt.Sprintf("line_%d", lineIdx)
					}

					if !existingPrompts[promptID] {
						var inTokens, outTokens, cacheRead, cacheCreate int64
						modelID := cand.LatestModelID
						if modelID == "" {
							modelID = "gemini-3.8-flash-high"
						}

						// Antigravity format
						if step.Type == "PLANNER_RESPONSE" || (step.InputTokens != nil || step.OutputTokens != nil) {
							if step.InputTokens != nil {
								inTokens = *step.InputTokens
							}
							if step.OutputTokens != nil {
								outTokens = *step.OutputTokens
							}
							if step.CacheReadTokens != nil {
								cacheRead = *step.CacheReadTokens
							}
						}

						// Claude Code format
						if step.Message != nil && step.Message.Usage != nil {
							inTokens = step.Message.Usage.InputTokens
							outTokens = step.Message.Usage.OutputTokens
							cacheRead = step.Message.Usage.CacheReadInputTokens
							cacheCreate = step.Message.Usage.CacheCreationInputTokens
							if step.Message.Model != "" {
								modelID = step.Message.Model
							}
						}

						// Only insert steps with non-zero tokens
						if inTokens > 0 || outTokens > 0 || cacheRead > 0 || cacheCreate > 0 {
							repAt := step.CreatedAt
							if repAt == "" {
								repAt = step.Timestamp
							}
							if repAt == "" {
								repAt = time.Now().UTC().Format(time.RFC3339Nano)
							} else if t, err := time.Parse(time.RFC3339, repAt); err == nil {
								repAt = t.UTC().Format(time.RFC3339Nano)
							}

							toInsert = append(toInsert, eventToInsert{
								SessionID:           cand.SessionID,
								NodeID:              cand.NodeID,
								PromptID:            promptID,
								ProductType:         cand.ProductType,
								ReportedAt:          repAt,
								ModelID:             modelID,
								ModelDisplayName:    "", // Leave empty so MAX(model_display_name) retains human-readable title
								CurrentInputTokens:  inTokens,
								CurrentOutputTokens: outTokens,
								CacheReadTokens:     cacheRead,
								CacheCreationTokens: cacheCreate,
							})
							existingPrompts[promptID] = true
						}
					}
				}
			}
		}

		if err != nil {
			break
		}
	}

	if len(toInsert) == 0 {
		return 0, nil
	}

	// Ensure node and session stubs exist
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = s.store.DB().ExecContext(ctx, "INSERT INTO nodes (node_id, os_type, first_seen_at, last_seen_at) VALUES (?, 'local', ?, ?) ON CONFLICT(node_id) DO NOTHING", cand.NodeID, nowStr, nowStr)
	_, _ = s.store.DB().ExecContext(ctx, "INSERT INTO sessions (session_id, node_id, product_type, cwd, transcript_path, latest_model_id, started_at, last_active_at) VALUES (?, ?, ?, '', ?, ?, ?, ?) ON CONFLICT(session_id) DO UPDATE SET transcript_path = excluded.transcript_path, latest_model_id = COALESCE(sessions.latest_model_id, excluded.latest_model_id)", cand.SessionID, cand.NodeID, cand.ProductType, cand.TranscriptPath, cand.LatestModelID, nowStr, nowStr)

	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	insertSQL := `
	INSERT INTO telemetry_events (
		node_id, session_id, prompt_id, product_type, reported_at,
		model_id, model_display_name, current_input_tokens, current_output_tokens,
		cache_read_tokens, cache_creation_tokens, raw_payload
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, e := range toInsert {
		rawPayload := fmt.Sprintf(`{"synced":true,"prompt_id":%q}`, e.PromptID)
		if _, err := stmt.ExecContext(ctx,
			e.NodeID, e.SessionID, e.PromptID, e.ProductType, e.ReportedAt,
			e.ModelID, e.ModelDisplayName, e.CurrentInputTokens, e.CurrentOutputTokens,
			e.CacheReadTokens, e.CacheCreationTokens, rawPayload,
		); err != nil {
			return 0, fmt.Errorf("failed to insert synced event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(toInsert), nil
}
