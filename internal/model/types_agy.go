package model

import "encoding/json"

// StatusLineData is the primary data structure for rendering the statusline.
type StatusLineData struct {
	CWD               string                 `json:"cwd"`
	SessionID         string                 `json:"session_id,omitempty"`
	ConversationID    string                 `json:"conversation_id,omitempty"`
	ConversationTitle string                 `json:"conversation_title,omitempty"`
	TranscriptPath    string                 `json:"transcript_path,omitempty"`
	Model             ModelInfo              `json:"model"`
	Workspace         WorkspaceInfo          `json:"workspace"`
	Version           string                 `json:"version,omitempty"`
	ContextWindow     ContextWindowInfo      `json:"context_window"`
	Exceeds200kTokens *bool                  `json:"exceeds_200k_tokens,omitempty"`
	Product           string                 `json:"product,omitempty"`
	Quota             map[string]QuotaBucket `json:"quota,omitempty"`
	AgentState        string                 `json:"agent_state,omitempty"`
	VCS               VCSInfo                `json:"vcs,omitempty"`
	PlanTier          string                 `json:"plan_tier,omitempty"`
	Email             string                 `json:"email,omitempty"`
	TerminalWidth     int                    `json:"terminal_width,omitempty"`
	Cost              CostInfo               `json:"cost,omitempty"`
}

type ContextWindowInfo struct {
	TotalInputTokens    int64   `json:"total_input_tokens"`
	TotalOutputTokens   int64   `json:"total_output_tokens"`
	ContextWindowSize   int64   `json:"context_window_size"`
	UsedPercentage      float64 `json:"used_percentage"`
	RemainingPercentage float64 `json:"remaining_percentage"`
}

func (c *ContextWindowInfo) UnmarshalJSON(b []byte) error {
	var size int64
	if err := json.Unmarshal(b, &size); err == nil {
		c.ContextWindowSize = size
		return nil
	}
	type alias ContextWindowInfo
	var aux alias
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*c = ContextWindowInfo(aux)
	return nil
}

type ModelInfo struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name"`
	Effort      string `json:"effort,omitempty"`
}

type WorkspaceInfo struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
	Repo       *struct {
		Host  string `json:"host,omitempty"`
		Owner string `json:"owner,omitempty"`
		Name  string `json:"name,omitempty"`
	} `json:"repo,omitempty"`
}

type QuotaBucket struct {
	RemainingFraction float64 `json:"remaining_fraction,omitempty"`
	RemainingAmount   float64 `json:"remaining_amount,omitempty"`
	ResetTime         string  `json:"reset_time,omitempty"`
	ResetInSeconds    int64   `json:"reset_in_seconds,omitempty"`
	Disabled          bool    `json:"disabled,omitempty"`
}

type VCSInfo struct {
	Type   string `json:"type,omitempty"`
	Branch string `json:"branch,omitempty"`
	Client string `json:"client,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
	Ahead  int    `json:"ahead,omitempty"`
	Behind int    `json:"behind,omitempty"`
}

type CostInfo struct {
	TotalCostUSD       float64 `json:"total_cost_usd,omitempty"`
	TotalUSD           float64 `json:"total_usd,omitempty"`
	TotalDurationMS    int64   `json:"total_duration_ms,omitempty"`
	TotalAPIDurationMS int64   `json:"total_api_duration_ms,omitempty"`
	TotalLinesAdded    int     `json:"total_lines_added,omitempty"`
	TotalLinesRemoved  int     `json:"total_lines_removed,omitempty"`
}
