package session

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ResolveTranscriptPath checks if a transcript file exists, or resolves it from local standard paths.
func ResolveTranscriptPath(productType, sessionID, currentPath string) string {
	if currentPath != "" {
		if fi, err := os.Stat(currentPath); err == nil && !fi.IsDir() {
			return currentPath
		}
	}
	if sessionID == "" {
		return ""
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}

	prod := strings.ToLower(productType)
	if strings.Contains(prod, "antigravity") || strings.Contains(prod, "agy") {
		p := filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", sessionID, ".system_generated", "logs", "transcript.jsonl")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
		matches, _ := filepath.Glob(filepath.Join(homeDir, ".gemini", "antigravity*", "brain", sessionID, ".system_generated", "logs", "transcript.jsonl"))
		if len(matches) > 0 {
			return matches[0]
		}
	} else if strings.Contains(prod, "claude") {
		matches, _ := filepath.Glob(filepath.Join(homeDir, ".claude", "projects", "*", sessionID+".jsonl"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// LoadTranscriptPreview scans the tail of a transcript file and extracts the latest
// User Prompt and Model Assistant completion.
func LoadTranscriptPreview(transcriptPath string) (userPrompt, assistantReply string, err error) {
	if transcriptPath == "" {
		return "", "", os.ErrNotExist
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	size := fi.Size()
	if size == 0 {
		return "", "", nil
	}

	// Read up to last 2MB to keep memory and CPU usage minimal
	var offset int64
	readLen := size
	if size > 2*1024*1024 {
		offset = size - 2*1024*1024
		readLen = 2 * 1024 * 1024
	}

	buf := make([]byte, readLen)
	_, err = f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return "", "", err
	}

	lines := bytes.Split(buf, []byte("\n"))

	// Traverse lines backwards to find latest Assistant and User turns
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		var generic map[string]interface{}
		if err := json.Unmarshal(line, &generic); err != nil {
			continue
		}

		// 1. Antigravity format
		if stepType, ok := generic["type"].(string); ok {
			switch stepType {
			case "PLANNER_RESPONSE":
				if assistantReply == "" {
					if content, ok := generic["content"].(string); ok && content != "" {
						assistantReply = cleanPreviewText(content)
					}
				}
			case "USER_INPUT":
				if userPrompt == "" {
					if content, ok := generic["content"].(string); ok && content != "" {
						userPrompt = cleanPreviewText(content)
					}
				}
			}
		}

		// 2. Claude Code format
		if cType, ok := generic["type"].(string); ok {
			switch cType {
			case "assistant":
				if assistantReply == "" {
					assistantReply = extractClaudeContent(generic)
				}
			case "user", "human":
				if userPrompt == "" {
					userPrompt = extractClaudeContent(generic)
				}
			}
		}

		if userPrompt != "" && assistantReply != "" {
			break
		}
	}

	return userPrompt, assistantReply, nil
}

func extractClaudeContent(obj map[string]interface{}) string {
	if msg, ok := obj["message"].(map[string]interface{}); ok {
		if content, ok := msg["content"].(string); ok {
			return cleanPreviewText(content)
		}
		if contentArr, ok := msg["content"].([]interface{}); ok {
			var sb strings.Builder
			for _, item := range contentArr {
				if block, ok := item.(map[string]interface{}); ok {
					if text, ok := block["text"].(string); ok {
						sb.WriteString(text)
						sb.WriteString(" ")
					}
				}
			}
			return cleanPreviewText(sb.String())
		}
	}
	if content, ok := obj["content"].(string); ok {
		return cleanPreviewText(content)
	}
	return ""
}

func cleanPreviewText(s string) string {
	s = strings.TrimSpace(s)
	// Remove excessive consecutive blank lines
	lines := strings.Split(s, "\n")
	var result []string
	for _, l := range lines {
		trimmed := strings.TrimRight(l, "\r \t")
		result = append(result, trimmed)
		if len(result) >= 15 { // cap preview at 15 lines
			result = append(result, "...")
			break
		}
	}
	cleaned := strings.Join(result, "\n")
	if len(cleaned) > 1200 {
		cleaned = cleaned[:1200] + "..."
	}
	return cleaned
}
