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

func TestFormatAccount(t *testing.T) {
	// Empty case
	if got := FormatAccount(nil); got != "" {
		t.Errorf("expected empty string for nil data, got %q", got)
	}
	emptyData := &model.StatusLineData{Email: ""}
	if got := FormatAccount(emptyData); got != "" {
		t.Errorf("expected empty string for empty email, got %q", got)
	}

	// Valid email
	data := &model.StatusLineData{Email: "kirito_wu@163.com"}
	got := FormatAccount(data)
	if StripANSI(got) != "<kirito_wu@163.com>" {
		t.Errorf("expected angle brackets <kirito_wu@163.com>, got %q", StripANSI(got))
	}

	// Compact prefix
	compact := formatAccountCompact(data)
	if StripANSI(compact) != "<kirito_wu>" {
		t.Errorf("expected compact prefix <kirito_wu>, got %q", StripANSI(compact))
	}
}

func TestAssembleLineWithAccount(t *testing.T) {
	data := &model.StatusLineData{
		CWD:   "/workspace/repo",
		Email: "kirito_wu@163.com",
		VCS: model.VCSInfo{
			Branch: "main",
		},
		Model: model.ModelInfo{
			DisplayName: "Gemini 3.8 Flash (High)",
		},
	}

	line := AssembleLine(data, "")
	plain := StripANSI(line)

	// Account must be present in angle brackets
	if !strings.Contains(plain, "<kirito_wu@163.com>") {
		t.Errorf("expected account in line, got %s", plain)
	}

	// Account should appear after VCS and before Model
	vcsIdx := strings.Index(plain, "(main)")
	accIdx := strings.Index(plain, "<kirito_wu@163.com>")
	modelIdx := strings.Index(plain, "[Gemini 3.8 Flash (High)]")

	if vcsIdx == -1 || accIdx == -1 || modelIdx == -1 {
		t.Fatalf("expected vcs, account, and model to all be present: %s", plain)
	}
	if !(vcsIdx < accIdx && accIdx < modelIdx) {
		t.Errorf("expected order vcs < account < model, got vcs=%d, acc=%d, model=%d in %s", vcsIdx, accIdx, modelIdx, plain)
	}
}

func TestModularSegmentsReordering(t *testing.T) {
	data := &model.StatusLineData{
		CWD:   "/workspace/repo",
		Email: "user@test.org",
		Model: model.ModelInfo{
			DisplayName: "Gemini 3.8 Flash (High)",
		},
		ContextWindow: model.ContextWindowInfo{
			TotalInputTokens:  1000,
			TotalOutputTokens: 200,
		},
	}

	// Custom order: tokens, account, dir
	customSegments := []string{"tokens", "account", "dir"}
	line := AssembleLine(data, "", customSegments)
	plain := StripANSI(line)

	tokIdx := strings.Index(plain, "↑1k")
	accIdx := strings.Index(plain, "<user@test.org>")
	dirIdx := strings.Index(plain, "[/workspace/repo]")

	if tokIdx == -1 || accIdx == -1 || dirIdx == -1 {
		t.Fatalf("expected tokens, account, and dir to be present: %s", plain)
	}
	if !(tokIdx < accIdx && accIdx < dirIdx) {
		t.Errorf("expected order tokens < account < dir, got %s", plain)
	}
	// Model was not in customSegments, so it must NOT be rendered
	if strings.Contains(plain, "Gemini") {
		t.Errorf("expected model omitted, got %s", plain)
	}
}

func TestModularSegmentsExplicitSeparator(t *testing.T) {
	data := &model.StatusLineData{
		CWD: "/workspace/repo",
		Model: model.ModelInfo{
			DisplayName: "Flash",
		},
	}

	customSegments := []string{"dir", "|", "model"}
	line := AssembleLine(data, "", customSegments)
	plain := StripANSI(line)

	expected := "[/workspace/repo] | [Flash]"
	if plain != expected {
		t.Errorf("expected %q, got %q", expected, plain)
	}
}
