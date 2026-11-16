package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"statusline/internal/storage"
)

type transcriptStep struct {
	StepIndex           *int64  `json:"step_index,omitempty"`
	Type                string  `json:"type,omitempty"`
	CreatedAt           string  `json:"created_at,omitempty"`
	Timestamp           string  `json:"timestamp,omitempty"`
	InputTokens         *int64  `json:"input_tokens,omitempty"`
	OutputTokens        *int64  `json:"output_tokens,omitempty"`
	CacheReadTokens     *int64  `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens *int64  `json:"cache_creation_tokens,omitempty"`
	UUID                string  `json:"uuid,omitempty"`
	MessageID           string  `json:"msg_id,omitempty"`
	Message             *struct {
		ID    string `json:"id,omitempty"`
		Model string `json:"model,omitempty"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`
}

type ScanSummary struct {
	FilesScanned int
	TotalEvents  int
	Events       []storage.SyncEventItem
}

// DiscoverAndScanTranscripts searches local Antigravity and Claude Code transcripts and extracts turn events.
func DiscoverAndScanTranscripts(homeDir, nodeID string) (*ScanSummary, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	summary := &ScanSummary{}
	filePaths := make(map[string]string) // path -> product_type

	// 1. Antigravity directories
	patternAGY := filepath.Join(homeDir, ".gemini", "antigravity*", "brain", "*", ".system_generated", "logs", "transcript.jsonl")
	matchesAGY, _ := filepath.Glob(patternAGY)
	for _, m := range matchesAGY {
		filePaths[m] = "antigravity"
	}

	// 2. Claude Code directories
	patternClaude := filepath.Join(homeDir, ".claude", "projects", "*", "*.jsonl")
	matchesClaude, _ := filepath.Glob(patternClaude)
	for _, m := range matchesClaude {
		filePaths[m] = "claude_code"
	}

	for p, productType := range filePaths {
		events, err := scanSingleTranscriptFile(p, productType, nodeID)
		if err == nil {
			summary.FilesScanned++
			summary.Events = append(summary.Events, events...)
		}
	}

	summary.TotalEvents = len(summary.Events)
	return summary, nil
}

func scanSingleTranscriptFile(filePath, productType, nodeID string) ([]storage.SyncEventItem, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sessionID := ""
	defaultModelID := "gemini-3.8-flash-high"
	if productType == "antigravity" {
		// ~/.gemini/antigravity*/brain/<conversation-id>/.system_generated/logs/transcript.jsonl
		sessionID = filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(filePath))))
	} else {
		// ~/.claude/projects/<project-id>/<session-id>.jsonl
		sessionID = strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
		defaultModelID = "claude-3-7-sonnet"
	}

	var events []storage.SyncEventItem
	seenPrompts := make(map[string]bool)
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
					} else if step.Message != nil && step.Message.ID != "" {
						promptID = step.Message.ID
					} else {
						promptID = fmt.Sprintf("line_%d", lineIdx)
					}

					if !seenPrompts[promptID] {
						var inTokens, outTokens, cacheRead, cacheCreate int64
						modelID := defaultModelID

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
							if step.CacheCreationTokens != nil {
								cacheCreate = *step.CacheCreationTokens
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

						if inTokens > 0 || outTokens > 0 || cacheRead > 0 || cacheCreate > 0 {
							repAt := step.CreatedAt
							if repAt == "" {
								repAt = step.Timestamp
							}
							if repAt == "" {
								repAt = time.Now().UTC().Format(time.RFC3339Nano)
							} else if t, parseErr := time.Parse(time.RFC3339, repAt); parseErr == nil {
								repAt = t.UTC().Format(time.RFC3339Nano)
							}

							events = append(events, storage.SyncEventItem{
								SessionID:           sessionID,
								NodeID:              nodeID,
								PromptID:            promptID,
								ProductType:         productType,
								ReportedAt:          repAt,
								ModelID:             modelID,
								ModelDisplayName:    "",
								CurrentInputTokens:  inTokens,
								CurrentOutputTokens: outTokens,
								CacheReadTokens:     cacheRead,
								CacheCreationTokens: cacheCreate,
							})
							seenPrompts[promptID] = true
						}
					}
				}
			}
		}

		if err != nil {
			break
		}
	}

	return events, nil
}
