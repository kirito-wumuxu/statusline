package model

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"statusline/internal/node"
)

type TelemetryEnvelope struct {
	NodeID        string                 `json:"node_id"`
	Hostname      string                 `json:"hostname"`
	OSType        string                 `json:"os_type"`
	OSArch        string                 `json:"os_arch"`
	ClientVersion string                 `json:"client_version"`
	ReportedAt    string                 `json:"reported_at"`
	Payload       map[string]interface{} `json:"payload"`
	Normalized    NormalizedData         `json:"normalized"`
}

type NormalizedData struct {
	ProductType         string               `json:"product_type"`
	SessionID           string               `json:"session_id"`
	PromptID            string               `json:"prompt_id"`
	ConversationID      string               `json:"conversation_id"`
	SessionName         string               `json:"session_name"`
	AccountEmail        string               `json:"account_email"`
	PlanTier            string               `json:"plan_tier"`
	ToolVersion         string               `json:"tool_version"`
	CWD                 string               `json:"cwd"`
	ProjectDir          string               `json:"project_dir"`
	TranscriptPath      string               `json:"transcript_path"`
	GitRepoHost         string               `json:"git_repo_host"`
	GitRepoOwner        string               `json:"git_repo_owner"`
	GitRepoName         string               `json:"git_repo_name"`
	GitBranch           string               `json:"git_branch"`
	GitWorktree         string               `json:"git_worktree"`
	ModelID             string               `json:"model_id"`
	ModelDisplayName    string               `json:"model_display_name"`
	ModelEffort         string               `json:"model_effort"`
	AgentState          string               `json:"agent_state"`
	TotalInputTokens    int64                `json:"total_input_tokens"`
	TotalOutputTokens   int64                `json:"total_output_tokens"`
	CurrentInputTokens  int64                `json:"current_input_tokens"`
	CurrentOutputTokens int64                `json:"current_output_tokens"`
	ContextWindowSize   int64                `json:"context_window_size"`
	UsedPercentage      float64              `json:"used_percentage"`
	RemainingPct        float64              `json:"remaining_percentage"`
	Exceeds200k         bool                 `json:"exceeds_200k_tokens"`
	TotalCostUSD        float64              `json:"total_cost_usd"`
	TotalDurationMS     int64                `json:"total_duration_ms"`
	TotalAPIDuration    int64                `json:"total_api_duration_ms"`
	TotalLinesAdded     int                  `json:"total_lines_added"`
	TotalLinesRemoved   int                  `json:"total_lines_removed"`
	CacheHitRatio       float64              `json:"cache_hit_ratio"`
	CacheCreationTokens int64                `json:"cache_creation_tokens"`
	CacheReadTokens     int64                `json:"cache_read_tokens"`
	Quotas              map[string]QuotaItem `json:"quotas,omitempty"`
}

type QuotaItem struct {
	RemainingFraction float64 `json:"remaining_fraction"`
	RemainingAmount   float64 `json:"remaining_amount"`
	ResetTime         string  `json:"reset_time"`
	ResetInSeconds    int64   `json:"reset_in_seconds"`
}

func ParseAndNormalize(raw []byte, n node.NodeInfo) (*TelemetryEnvelope, *StatusLineData, error) {
	// Clean invalid UTF-8 bytes upfront to ensure json.Unmarshal never fails on corrupted bytes
	if len(raw) > 0 {
		raw = []byte(CleanUTF8(string(raw)))
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	envelope := &TelemetryEnvelope{
		NodeID:        n.NodeID,
		Hostname:      n.Hostname,
		OSType:        n.OSType,
		OSArch:        n.OSArch,
		ClientVersion: n.ClientVersion,
		ReportedAt:    nowStr,
		Payload:       make(map[string]interface{}),
	}

	if len(raw) == 0 {
		data := &StatusLineData{
			Model: ModelInfo{DisplayName: "Gemini", ID: "gemini"},
			ContextWindow: ContextWindowInfo{
				ContextWindowSize: 1048576,
			},
		}
		envelope.Normalized = NormalizedData{
			ProductType:       "antigravity",
			ModelID:           "gemini",
			ModelDisplayName:  "Gemini",
			ContextWindowSize: 1048576,
		}
		return envelope, data, nil
	}

	// Try unmarshaling raw map
	_ = json.Unmarshal(raw, &envelope.Payload)

	// Check if Antigravity payload
	var agy StatusLineData
	_ = json.Unmarshal(raw, &agy)

	// Check if Claude Code payload
	var cc ClaudeCodePayload
	_ = json.Unmarshal(raw, &cc)

	// Detect product type:
	// AGY explicitly sets product="antigravity"
	isAGY := agy.Product == "antigravity" || (agy.Model.ID != "" && agy.Model.DisplayName != "" && cc.Cost == nil && cc.PromptID == "")

	if isAGY {
		norm := normalizeAGY(&agy)
		envelope.Normalized = norm
		return envelope, &agy, nil
	}

	// Otherwise treat as Claude Code
	norm, statusData := normalizeClaude(&cc, raw)
	envelope.Normalized = norm
	return envelope, statusData, nil
}

func normalizeAGY(agy *StatusLineData) NormalizedData {
	cwd := CleanUTF8(agy.CWD)
	if agy.Workspace.CurrentDir != "" {
		cwd = CleanUTF8(agy.Workspace.CurrentDir)
	}

	modelID := agy.Model.ID
	if modelID == "" {
		modelID = "gemini"
	}
	displayName := agy.Model.DisplayName
	if displayName == "" {
		displayName = modelID
	}

	effort := agy.Model.Effort

	norm := NormalizedData{
		ProductType:         "antigravity",
		SessionID:           agy.SessionID,
		ConversationID:      agy.ConversationID,
		AccountEmail:        agy.Email,
		PlanTier:            agy.PlanTier,
		ToolVersion:         agy.Version,
		CWD:                 cwd,
		ProjectDir:          CleanUTF8(agy.Workspace.ProjectDir),
		TranscriptPath:      CleanUTF8(agy.TranscriptPath),
		GitBranch:           agy.VCS.Branch,
		ModelID:             modelID,
		ModelDisplayName:    displayName,
		ModelEffort:         effort,
		AgentState:          agy.AgentState,
		TotalInputTokens:    agy.ContextWindow.TotalInputTokens,
		TotalOutputTokens:   agy.ContextWindow.TotalOutputTokens,
		ContextWindowSize:   agy.ContextWindow.ContextWindowSize,
		UsedPercentage:      agy.ContextWindow.UsedPercentage,
		RemainingPct:        agy.ContextWindow.RemainingPercentage,
		Exceeds200k:         agy.Exceeds200kTokens != nil && *agy.Exceeds200kTokens,
		TotalCostUSD:        agy.Cost.TotalUSD,
		TotalDurationMS:     agy.Cost.TotalDurationMS,
		TotalAPIDuration:    agy.Cost.TotalAPIDurationMS,
		TotalLinesAdded:     agy.Cost.TotalLinesAdded,
		TotalLinesRemoved:   agy.Cost.TotalLinesRemoved,
		Quotas:              make(map[string]QuotaItem),
	}

	if agy.ContextWindow.CurrentUsage != nil {
		norm.CurrentInputTokens = agy.ContextWindow.CurrentUsage.InputTokens
		norm.CurrentOutputTokens = agy.ContextWindow.CurrentUsage.OutputTokens
		norm.CacheReadTokens = agy.ContextWindow.CurrentUsage.CacheReadInputTokens
		norm.CacheCreationTokens = agy.ContextWindow.CurrentUsage.CacheCreationInputTokens
	}

	if norm.ContextWindowSize == 0 {
		norm.ContextWindowSize = 1048576
	}

	if agy.Workspace.Repo != nil {
		norm.GitRepoHost = agy.Workspace.Repo.Host
		norm.GitRepoOwner = agy.Workspace.Repo.Owner
		norm.GitRepoName = agy.Workspace.Repo.Name
	}

	for k, b := range agy.Quota {
		norm.Quotas[k] = QuotaItem{
			RemainingFraction: b.RemainingFraction,
			RemainingAmount:   b.RemainingAmount,
			ResetTime:         b.ResetTime,
			ResetInSeconds:    b.ResetInSeconds,
		}
	}

	return norm
}

func normalizeClaude(cc *ClaudeCodePayload, raw []byte) (NormalizedData, *StatusLineData) {
	sessionName := CleanUTF8(cc.SessionName)
	cwd := CleanUTF8(cc.CWD)

	modelID := cc.Model.ID
	if modelID == "" {
		modelID = "claude-3-7-sonnet"
	}
	displayName := cc.Model.DisplayName
	if displayName == "" {
		displayName = modelID
	}

	effort := cc.Model.Effort
	if effort == "" && cc.Effort != nil {
		effort = cc.Effort.Level
	}

	norm := NormalizedData{
		ProductType:    "claude_code",
		SessionID:      cc.SessionID,
		PromptID:       cc.PromptID,
		SessionName:    sessionName,
		ToolVersion:    cc.Version,
		CWD:            cwd,
		TranscriptPath: CleanUTF8(cc.TranscriptPath),
		ModelID:        modelID,
		ModelDisplayName: displayName,
		ModelEffort:    effort,
		AccountEmail:   cc.Email,
		AgentState:     "active",
		Quotas:         make(map[string]QuotaItem),
	}

	if cc.Workspace != nil {
		norm.ProjectDir = CleanUTF8(cc.Workspace.ProjectDir)
		if cc.Workspace.Repo != nil {
			norm.GitRepoHost = cc.Workspace.Repo.Host
			norm.GitRepoOwner = cc.Workspace.Repo.Owner
			norm.GitRepoName = cc.Workspace.Repo.Name
		}
	}

	if cc.Worktree != nil {
		norm.GitBranch = cc.Worktree.Branch
		norm.GitWorktree = cc.Worktree.Name
	}

	if cc.ContextWindow != nil {
		norm.TotalInputTokens = cc.ContextWindow.TotalInputTokens
		norm.TotalOutputTokens = cc.ContextWindow.TotalOutputTokens
		norm.ContextWindowSize = cc.ContextWindow.ContextWindowSize
		norm.UsedPercentage = cc.ContextWindow.UsedPercentage
		norm.RemainingPct = cc.ContextWindow.RemainingPercentage
	}
	if norm.ContextWindowSize == 0 {
		norm.ContextWindowSize = 200000
	}
	if norm.UsedPercentage == 0 && norm.ContextWindowSize > 0 && (norm.TotalInputTokens+norm.TotalOutputTokens) > 0 {
		norm.UsedPercentage = math.Round(float64(norm.TotalInputTokens+norm.TotalOutputTokens)/float64(norm.ContextWindowSize)*10000) / 100
	}

	if cc.Cost != nil {
		costUSD := cc.Cost.TotalCostUSD
		if costUSD == 0 {
			costUSD = cc.Cost.TotalUSD
		}
		norm.TotalCostUSD = costUSD
		norm.TotalDurationMS = cc.Cost.TotalDurationMS
		norm.TotalAPIDuration = cc.Cost.TotalAPIDurationMS
		norm.TotalLinesAdded = cc.Cost.TotalLinesAdded
		norm.TotalLinesRemoved = cc.Cost.TotalLinesRemoved
	}

	if cc.PromptCache != nil {
		norm.CacheHitRatio = cc.PromptCache.HitRatio
		norm.CacheCreationTokens = cc.PromptCache.CacheCreationTokens
		norm.CacheReadTokens = cc.PromptCache.CacheReadTokens
	}

	// Also build a StatusLineData object so render package can format Claude Code sessions
	statusData := &StatusLineData{
		CWD:       cwd,
		SessionID: cc.SessionID,
		Model: ModelInfo{
			ID:          modelID,
			DisplayName: displayName,
			Effort:      effort,
		},
		ContextWindow: ContextWindowInfo{
			TotalInputTokens:    norm.TotalInputTokens,
			TotalOutputTokens:   norm.TotalOutputTokens,
			ContextWindowSize:   norm.ContextWindowSize,
			UsedPercentage:      norm.UsedPercentage,
			RemainingPercentage: norm.RemainingPct,
		},
		VCS: VCSInfo{
			Branch: norm.GitBranch,
		},
		Email: cc.Email,
		Cost: CostInfo{
			TotalCostUSD: norm.TotalCostUSD,
			TotalUSD:     norm.TotalCostUSD,
		},
	}

	return norm, statusData
}

// ExtractQuotasSummary groups raw quota buckets into sorted slices.
type QuotaBucketGroup struct {
	RemainingPct int
	ResetSeconds int64
}

func ExtractQuotasSummary(quotas map[string]QuotaBucket) (gemini, claude []QuotaBucketGroup) {
	for k, b := range quotas {
		lower := strings.ToLower(k)
		pct := int(math.Round(b.RemainingFraction * 100))
		item := QuotaBucketGroup{RemainingPct: pct, ResetSeconds: b.ResetInSeconds}
		if strings.Contains(lower, "gemini") {
			gemini = append(gemini, item)
		} else if strings.Contains(lower, "claude") || strings.Contains(lower, "3p") || strings.Contains(lower, "sonnet") {
			claude = append(claude, item)
		}
	}
	sort.Slice(gemini, func(i, j int) bool { return gemini[i].ResetSeconds < gemini[j].ResetSeconds })
	sort.Slice(claude, func(i, j int) bool { return claude[i].ResetSeconds < claude[j].ResetSeconds })
	return gemini, claude
}


type ModelSummary struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Effort      string `json:"effort,omitempty"`
}

type AccountSummary struct {
	Email    string `json:"email,omitempty"`
	PlanTier string `json:"plan_tier,omitempty"`
}

type TokensSummary struct {
	TotalInputTokens  int64 `json:"total_input_tokens"`
	TotalOutputTokens int64 `json:"total_output_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
}

type ContextSummary struct {
	ContextWindowSize int64   `json:"context_window_size"`
	UsedPercentage    float64 `json:"used_percentage"`
}

type QuotaSummary struct {
	Gemini ProviderQuota `json:"gemini"`
	Claude ProviderQuota `json:"claude"`
}

type ProviderQuota struct {
	ShortPercentage int    `json:"short_percentage"`
	ShortReset      string `json:"short_reset"`
	LongPercentage  int    `json:"long_percentage"`
	LongReset       string `json:"long_reset"`
}

// SplitQuotaKeySuffix splits a full quota_key into prefix (without duration suffix) and window label:
// e.g. "gemini-5h" -> ("gemini", "5h")
// e.g. "gemini-argon-wtu-weekly" -> ("gemini-argon-wtu", "7d")
// e.g. "3p-5h" -> ("3p", "5h")
// e.g. "deepseek-daily" -> ("deepseek", "24h")
func SplitQuotaKeySuffix(key string) (prefix string, window string) {
	lower := strings.ToLower(key)
	suffixes := []struct {
		suffix string
		window string
	}{
		{"-5h", "5h"},
		{"-weekly", "7d"},
		{"-daily", "24h"},
		{"-monthly", "30d"},
		{"-short", "5h"},
	}
	for _, s := range suffixes {
		if strings.HasSuffix(lower, s.suffix) {
			return key[:len(key)-len(s.suffix)], s.window
		}
	}
	return key, ""
}

// DisplayAliasItem represents a single alias mapping rule.
type DisplayAliasItem struct {
	Category    string `json:"category,omitempty"` // "quota", "model", "account"
	Key         string `json:"key"`                // prefix or pattern, e.g. "gemini", "gemini-argon", "3p"
	DisplayName string `json:"name"`               // display name for full mode
	ShortName   string `json:"short_name"`         // short name for compact mode
	SortOrder   int    `json:"sort_order,omitempty"`// display order (1, 2, 3...)
}

// DisplayAliasesConfig groups all alias mapping rules.
type DisplayAliasesConfig struct {
	Quotas   []DisplayAliasItem `json:"quotas"`
	Models   []DisplayAliasItem `json:"models"`
	Accounts []DisplayAliasItem `json:"accounts"`
}

func FormatDurationCompact(seconds int64) string {
	if seconds <= 0 {
		return "0m"
	}
	days := seconds / 86400
	remSeconds := seconds % 86400
	hours := remSeconds / 3600
	remMinutes := (remSeconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, remMinutes)
	}
	return fmt.Sprintf("%dm", remMinutes)
}
