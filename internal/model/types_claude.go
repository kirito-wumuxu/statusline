package model

// ClaudeCodePayload maps the JSON emitted by Claude Code statusline integrations.
type ClaudeCodePayload struct {
	SessionID      string                `json:"session_id,omitempty"`
	PromptID       string                `json:"prompt_id,omitempty"`
	SessionName    string                `json:"session_name,omitempty"`
	TranscriptPath string                `json:"transcript_path,omitempty"`
	CWD            string                `json:"cwd,omitempty"`
	Version        string                `json:"version,omitempty"`
	Model          ClaudeModelInfo       `json:"model,omitempty"`
	Effort         *ClaudeEffortInfo     `json:"effort,omitempty"`
	Cost           *CostInfo             `json:"cost,omitempty"`
	Workspace      *ClaudeWorkspaceInfo  `json:"workspace,omitempty"`
	Worktree       *ClaudeWorktreeInfo   `json:"worktree,omitempty"`
	ContextWindow  *ContextWindowInfo    `json:"context_window,omitempty"`
	PromptCache    *ClaudePromptCache    `json:"prompt_cache,omitempty"`
	Email          string                `json:"email,omitempty"`
}

type ClaudeModelInfo struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Effort      string `json:"effort,omitempty"`
}

type ClaudeEffortInfo struct {
	Level string `json:"level,omitempty"`
}

type ClaudeWorkspaceInfo struct {
	CurrentDir string          `json:"current_dir,omitempty"`
	ProjectDir string          `json:"project_dir,omitempty"`
	Repo       *ClaudeRepoInfo `json:"repo,omitempty"`
}

type ClaudeRepoInfo struct {
	Host  string `json:"host,omitempty"`
	Owner string `json:"owner,omitempty"`
	Name  string `json:"name,omitempty"`
}

type ClaudeWorktreeInfo struct {
	Name   string `json:"name,omitempty"`
	Branch string `json:"branch,omitempty"`
}

type ClaudePromptCache struct {
	HitRatio            float64 `json:"hit_ratio,omitempty"`
	CacheCreationTokens int64   `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64   `json:"cache_read_tokens,omitempty"`
}
