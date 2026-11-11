package render

import (
	"strings"
	"testing"

	"statusline/internal/model"
)

func TestStripANSI(t *testing.T) {
	colored := "\033[1;32m[Gemini]\033[0m \033[90m|\033[0m"
	plain := StripANSI(colored)
	if plain != "[Gemini] |" {
		t.Fatalf("expected '[Gemini] |', got %q", plain)
	}
}

func TestAssembleLineStandard(t *testing.T) {
	data := &model.StatusLineData{
		CWD: "/home/user/workspace/app",
		Model: model.ModelInfo{
			DisplayName: "Gemini 3.8 Flash (High)",
		},
		ContextWindow: model.ContextWindowInfo{
			ContextWindowSize: 1048576,
			UsedPercentage:    25.0,
		},
		VCS: model.VCSInfo{
			Branch: "main",
		},
		Quota: map[string]model.QuotaBucket{
			"gemini-weekly": {
				RemainingFraction: 0.90,
				ResetInSeconds:    7200,
			},
		},
	}

	line := AssembleLine(data, "/home/user")
	plain := StripANSI(line)

	if !strings.Contains(plain, "[~/workspace/app]") {
		t.Errorf("expected collapsed home dir, got %s", plain)
	}
	if !strings.Contains(plain, "(main)") {
		t.Errorf("expected branch (main), got %s", plain)
	}
	if !strings.Contains(plain, "[Gemini 3.8 Flash (High)]") {
		t.Errorf("expected model name, got %s", plain)
	}
	if !strings.Contains(plain, "25%") {
		t.Errorf("expected context percentage, got %s", plain)
	}
}

func TestAdaptiveCompaction(t *testing.T) {
	data := &model.StatusLineData{
		CWD: "/home/user/very/long/nested/directory/path/here",
		Model: model.ModelInfo{
			DisplayName: "Gemini 3.8 Flash (High)",
		},
		ContextWindow: model.ContextWindowInfo{
			ContextWindowSize: 1048576,
			UsedPercentage:    25.0,
		},
		TerminalWidth: 50, // Narrow terminal (Review Focus #5)
	}

	line := AssembleLine(data, "/home/user")
	plain := StripANSI(line)

	if len(plain) > 55 {
		t.Errorf("expected compact line <= 55 chars, got length %d: %s", len(plain), plain)
	}
	// Under level 2 or 3, long model name is compacted to short like [Flash]
	if !strings.Contains(plain, "[Flash]") {
		t.Errorf("expected compact model [Flash], got %s", plain)
	}
}
