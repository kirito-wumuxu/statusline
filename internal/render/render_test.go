package render

import (
	"strings"
	"testing"

	"statusline/internal/config"
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

func TestFormatQuotaEmptyWhenNoData(t *testing.T) {
	// 1. Nil data
	if got := FormatQuota(nil); got != "" {
		t.Errorf("expected empty string for nil data, got %q", got)
	}

	// 2. Data without Quota map
	data := &model.StatusLineData{
		CWD: "/tmp",
		Model: model.ModelInfo{DisplayName: "MiniMax-M3"},
	}
	if got := FormatQuota(data); got != "" {
		t.Errorf("expected empty string when Quota is nil, got %q", got)
	}

	// 3. Data with empty Quota map
	data.Quota = make(map[string]model.QuotaBucket)
	if got := FormatQuota(data); got != "" {
		t.Errorf("expected empty string when Quota map is empty, got %q", got)
	}

	// 4. AssembleLine does not output quota or trailing pipe when Quota is absent (e.g. Claude Code with MiniMax)
	line := AssembleLine(data, "")
	plain := StripANSI(line)
	if strings.Contains(plain, "Gemini") || strings.Contains(plain, "Claude") {
		t.Errorf("expected no Gemini or Claude quota in line without quota data, got: %s", plain)
	}
	if strings.HasSuffix(plain, "|") {
		t.Errorf("expected no trailing pipe in line, got: %s", plain)
	}
}

func TestFormatQuotaProviderFiltering(t *testing.T) {
	// Only Gemini
	dataGemini := &model.StatusLineData{
		Quota: map[string]model.QuotaBucket{
			"gemini-flash": {RemainingFraction: 0.85, ResetInSeconds: 7200},
		},
	}
	plainGemini := StripANSI(FormatQuota(dataGemini))
	if !strings.Contains(plainGemini, "Gemini: 85%(2h0m)") {
		t.Errorf("expected Gemini quota, got %q", plainGemini)
	}
	if strings.Contains(plainGemini, "Claude") {
		t.Errorf("expected no Claude when only Gemini data exists, got %q", plainGemini)
	}

	// Only Claude
	dataClaude := &model.StatusLineData{
		Quota: map[string]model.QuotaBucket{
			"claude-3-7-sonnet": {RemainingFraction: 0.95, ResetInSeconds: 18000},
		},
	}
	plainClaude := StripANSI(FormatQuota(dataClaude))
	if !strings.Contains(plainClaude, "Claude: 95%(5h0m)") {
		t.Errorf("expected Claude quota, got %q", plainClaude)
	}
	if strings.Contains(plainClaude, "Gemini") {
		t.Errorf("expected no Gemini when only Claude data exists, got %q", plainClaude)
	}

	// Both Gemini and Claude
	dataBoth := &model.StatusLineData{
		Quota: map[string]model.QuotaBucket{
			"gemini-flash":      {RemainingFraction: 0.85, ResetInSeconds: 7200},
			"claude-3-7-sonnet": {RemainingFraction: 0.95, ResetInSeconds: 18000},
		},
	}
	plainBoth := StripANSI(FormatQuota(dataBoth))
	if !strings.Contains(plainBoth, "Gemini: 85%(2h0m)") || !strings.Contains(plainBoth, "Claude: 95%(5h0m)") || !strings.Contains(plainBoth, "|") {
		t.Errorf("expected both Gemini and Claude separated by pipe, got %q", plainBoth)
	}
}

func TestDisplayAliasesAndDynamicQuotas(t *testing.T) {
	// Configure test aliases
	SetAliases(config.DisplayAliasesConfig{
		Quotas: []config.DisplayAliasItem{
			{Key: "gemini", Name: "Gemini", ShortName: "G"},
			{Key: "gemini-argon", Name: "Argon", ShortName: "Ar"},
			{Key: "3p", Name: "Claude", ShortName: "C"},
		},
		Models: []config.DisplayAliasItem{
			{Key: "gemini-3.8-flash", Name: "Gemini 3.8 Flash", ShortName: "Flash"},
			{Key: "claude-3-7-sonnet", Name: "Claude 3.7 Sonnet", ShortName: "Sonnet"},
		},
		Accounts: []config.DisplayAliasItem{
			{Key: "dev@example.com", Name: "DevTeam", ShortName: "Dev"},
		},
	})

	// Test Quota with Argon and 3p dynamic keys
	data := &model.StatusLineData{
		Email: "dev@example.com",
		Model: model.ModelInfo{
			ID:          "claude-3-7-sonnet",
			DisplayName: "",
		},
		Quota: map[string]model.QuotaBucket{
			"gemini-5h":               {RemainingFraction: 0.90, ResetInSeconds: 7200},
			"gemini-argon-wtu-weekly": {RemainingFraction: 1.0, ResetInSeconds: 360000},
			"3p-5h":                   {RemainingFraction: 0.80, ResetInSeconds: 14400},
		},
	}

	// 1. Wide screen rendering: Gemini, Argon, Claude
	plainWide := StripANSI(FormatQuota(data))
	if !strings.Contains(plainWide, "Gemini: 90%(2h0m)") {
		t.Errorf("expected Gemini in wide quota, got %q", plainWide)
	}
	if !strings.Contains(plainWide, "Argon: 100%(4d4h)") {
		t.Errorf("expected Argon in wide quota, got %q", plainWide)
	}
	if !strings.Contains(plainWide, "Claude: 80%(4h0m)") {
		t.Errorf("expected Claude in wide quota, got %q", plainWide)
	}

	// Check provider ordering: Gemini -> Argon -> Claude
	gIdx := strings.Index(plainWide, "Gemini")
	aIdx := strings.Index(plainWide, "Argon")
	cIdx := strings.Index(plainWide, "Claude")
	if !(gIdx < aIdx && aIdx < cIdx) {
		t.Errorf("expected ordering Gemini < Argon < Claude, got g=%d, a=%d, c=%d in %q", gIdx, aIdx, cIdx, plainWide)
	}

	// 2. Compact mode rendering: G:, Ar:, C:
	plainCompact := StripANSI(formatQuotaCompact(data, false))
	if !strings.Contains(plainCompact, "G: 90%(2h0m)") {
		t.Errorf("expected G: in compact quota, got %q", plainCompact)
	}
	if !strings.Contains(plainCompact, "Ar: 100%(4d4h)") {
		t.Errorf("expected Ar: in compact quota, got %q", plainCompact)
	}
	if !strings.Contains(plainCompact, "C: 80%(4h0m)") {
		t.Errorf("expected C: in compact quota, got %q", plainCompact)
	}

	// 3. Model alias with empty DisplayName falls back to alias Name
	plainModel := StripANSI(FormatModel(data))
	if !strings.Contains(plainModel, "[Claude 3.7 Sonnet]") {
		t.Errorf("expected [Claude 3.7 Sonnet] for model without display name, got %q", plainModel)
	}

	// 4. Compact model uses short_name: [Sonnet]
	plainModelCompact := StripANSI(formatModelCompact(data))
	if !strings.Contains(plainModelCompact, "[Sonnet]") {
		t.Errorf("expected [Sonnet] in compact model, got %q", plainModelCompact)
	}

	// 5. Account alias: wide uses Name "DevTeam", compact uses ShortName "Dev"
	plainAccount := StripANSI(FormatAccount(data))
	if plainAccount != "<DevTeam>" {
		t.Errorf("expected <DevTeam> in wide account, got %q", plainAccount)
	}
	plainAccountCompact := StripANSI(formatAccountCompact(data))
	if plainAccountCompact != "<Dev>" {
		t.Errorf("expected <Dev> in compact account, got %q", plainAccountCompact)
	}
}
