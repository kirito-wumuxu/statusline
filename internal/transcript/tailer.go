package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TurnUsage represents atomic tokens consumed in one or more conversation turns.
type TurnUsage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

type transcriptLine struct {
	// Antigravity format
	StepIndex       *int64 `json:"step_index,omitempty"`
	Type            string `json:"type,omitempty"`
	InputTokens     *int64 `json:"input_tokens,omitempty"`
	OutputTokens    *int64 `json:"output_tokens,omitempty"`
	CacheReadTokens *int64 `json:"cache_read_tokens,omitempty"`

	// Claude Code format
	Message *struct {
		Model string `json:"model,omitempty"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`

	// Generic usage wrapper
	Usage *struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadTokens          int64 `json:"cache_read_tokens"`
	} `json:"usage,omitempty"`
}

// sanitizeSessionID replaces path-unsafe characters in sessionID.
func sanitizeSessionID(s string) string {
	if s == "" {
		return "default"
	}
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// getOffsetFilePath returns the path to the temporary offset tracking file.
func getOffsetFilePath(sessionID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf(".statusline_tail_%s.offset", sanitizeSessionID(sessionID)))
}

// readLastOffset reads the stored byte offset from disk.
func readLastOffset(sessionID string) int64 {
	offsetPath := getOffsetFilePath(sessionID)
	data, err := os.ReadFile(offsetPath)
	if err != nil {
		return 0
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

// writeLastOffset writes the updated byte offset to disk.
func writeLastOffset(sessionID string, offset int64) {
	offsetPath := getOffsetFilePath(sessionID)
	_ = os.WriteFile(offsetPath, []byte(strconv.FormatInt(offset, 10)), 0600)
}

// ProbeRecentUsage inspects the tail of transcriptPath starting from the last known offset.
// It executes with a strict timeout to ensure terminal rendering is never delayed.
func ProbeRecentUsage(transcriptPath string, sessionID string, timeout time.Duration) TurnUsage {
	if transcriptPath == "" {
		return TurnUsage{}
	}
	if timeout <= 0 {
		timeout = 5 * time.Millisecond
	}

	resultChan := make(chan TurnUsage, 1)

	go func() {
		resultChan <- probe(transcriptPath, sessionID)
	}()

	select {
	case res := <-resultChan:
		return res
	case <-time.After(timeout):
		return TurnUsage{}
	}
}

func probe(transcriptPath string, sessionID string) TurnUsage {
	stat, err := os.Stat(transcriptPath)
	if err != nil {
		return TurnUsage{}
	}

	fileSize := stat.Size()
	lastOffset := readLastOffset(sessionID)

	// File was truncated or rotated
	if fileSize < lastOffset {
		lastOffset = 0
	}

	// No new data appended
	if fileSize == lastOffset {
		return TurnUsage{}
	}

	file, err := os.Open(transcriptPath)
	if err != nil {
		return TurnUsage{}
	}
	defer file.Close()

	if _, err := file.Seek(lastOffset, io.SeekStart); err != nil {
		return TurnUsage{}
	}

	reader := bufio.NewReader(file)
	currentOffset := lastOffset
	var usage TurnUsage

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) > 0 {
			currentOffset += int64(len(lineBytes))
			trimmed := bytes.TrimSpace(lineBytes)
			if len(trimmed) > 0 {
				var tl transcriptLine
				if jsonErr := json.Unmarshal(trimmed, &tl); jsonErr == nil {
					// 1. Antigravity format
					if tl.Type == "PLANNER_RESPONSE" || (tl.InputTokens != nil || tl.OutputTokens != nil) {
						if tl.InputTokens != nil {
							usage.InputTokens += *tl.InputTokens
						}
						if tl.OutputTokens != nil {
							usage.OutputTokens += *tl.OutputTokens
						}
						if tl.CacheReadTokens != nil {
							usage.CacheReadTokens += *tl.CacheReadTokens
						}
					}

					// 2. Claude Code format
					if tl.Message != nil && tl.Message.Usage != nil {
						usage.InputTokens += tl.Message.Usage.InputTokens
						usage.OutputTokens += tl.Message.Usage.OutputTokens
						usage.CacheReadTokens += tl.Message.Usage.CacheReadInputTokens
						usage.CacheCreationTokens += tl.Message.Usage.CacheCreationInputTokens
					}

					// 3. Generic usage wrapper
					if tl.Usage != nil {
						usage.InputTokens += tl.Usage.InputTokens
						usage.OutputTokens += tl.Usage.OutputTokens
						cacheRead := tl.Usage.CacheReadInputTokens
						if cacheRead == 0 {
							cacheRead = tl.Usage.CacheReadTokens
						}
						usage.CacheReadTokens += cacheRead
						usage.CacheCreationTokens += tl.Usage.CacheCreationInputTokens
					}
				}
			}
		}

		if err != nil {
			break
		}
	}

	writeLastOffset(sessionID, currentOffset)
	return usage
}
