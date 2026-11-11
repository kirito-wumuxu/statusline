package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"statusline/internal/model"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex // serialize writes to avoid WAL table lock contention under high concurrency
}

type NodeRecord struct {
	NodeID        string `json:"node_id"`
	Hostname      string `json:"hostname"`
	OSType        string `json:"os_type"`
	OSArch        string `json:"os_arch"`
	IPAddress     string `json:"ip_address"`
	ClientVersion string `json:"client_version"`
	FirstSeenAt   string `json:"first_seen_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

type SessionRecord struct {
	SessionID          string  `json:"session_id"`
	NodeID             string  `json:"node_id"`
	ProductType        string  `json:"product_type"`
	ConversationID     string  `json:"conversation_id,omitempty"`
	SessionName        string  `json:"session_name,omitempty"`
	AccountEmail       string  `json:"account_email,omitempty"`
	PlanTier           string  `json:"plan_tier,omitempty"`
	ToolVersion        string  `json:"tool_version,omitempty"`
	CWD                string  `json:"cwd"`
	ProjectDir         string  `json:"project_dir,omitempty"`
	TranscriptPath     string  `json:"transcript_path,omitempty"`
	GitRepoHost        string  `json:"git_repo_host,omitempty"`
	GitRepoOwner       string  `json:"git_repo_owner,omitempty"`
	GitRepoName        string  `json:"git_repo_name,omitempty"`
	GitBranch          string  `json:"git_branch,omitempty"`
	GitWorktree        string  `json:"git_worktree,omitempty"`
	LatestModelID      string  `json:"latest_model_id"`
	TotalInputTokens   int64   `json:"total_input_tokens"`
	TotalOutputTokens  int64   `json:"total_output_tokens"`
	ContextUsedPct     float64 `json:"context_used_pct"`
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMS    int64   `json:"total_duration_ms"`
	TotalLinesAdded    int     `json:"total_lines_added"`
	TotalLinesRemoved  int     `json:"total_lines_removed"`
	StartedAt          string  `json:"started_at"`
	LastActiveAt       string  `json:"last_active_at"`
}

type EventRecord struct {
	ID                  int64   `json:"id"`
	NodeID              string  `json:"node_id"`
	SessionID           string  `json:"session_id"`
	PromptID            string  `json:"prompt_id,omitempty"`
	ProductType         string  `json:"product_type"`
	ReportedAt          string  `json:"reported_at"`
	ModelID             string  `json:"model_id"`
	ModelDisplayName    string  `json:"model_display_name,omitempty"`
	ModelEffort         string  `json:"model_effort,omitempty"`
	AgentState          string  `json:"agent_state,omitempty"`
	ContextWindowSize   int64   `json:"context_window_size"`
	TotalInputTokens    int64   `json:"total_input_tokens"`
	TotalOutputTokens   int64   `json:"total_output_tokens"`
	CurrentInputTokens  int64   `json:"current_input_tokens"`
	CurrentOutputTokens int64   `json:"current_output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	UsedPercentage      float64 `json:"used_percentage"`
	RemainingPct        float64 `json:"remaining_percentage"`
	Exceeds200k         bool    `json:"exceeds_200k_tokens"`
	TotalCostUSD        float64 `json:"total_cost_usd"`
	TotalDurationMS     int64   `json:"total_duration_ms"`
	TotalAPIDuration    int64   `json:"total_api_duration_ms"`
	CacheHitRatio       float64 `json:"cache_hit_ratio"`
	QuotaSnapshot       string  `json:"quota_snapshot,omitempty"`
	PromptCacheDetail   string  `json:"prompt_cache_detail,omitempty"`
}

type QuotaRecord struct {
	AccountEmail      string  `json:"account_email"`
	QuotaKey          string  `json:"quota_key"`
	RemainingFraction float64 `json:"remaining_fraction"`
	ResetTime         string  `json:"reset_time"`
	ResetInSeconds    int64   `json:"reset_in_seconds"`
	LastNodeID        string  `json:"last_node_id"`
	UpdatedAt         string  `json:"updated_at"`
}

type OverviewData struct {
	ActiveNodes       int           `json:"active_nodes"`
	ActiveSessions    int           `json:"active_sessions"`
	TotalInputTokens  int64         `json:"total_input_tokens"`
	TotalOutputTokens int64         `json:"total_output_tokens"`
	TotalTokens       int64         `json:"total_tokens"`
	TotalCostUSD      float64       `json:"total_cost_usd"`
	TodayEventsCount  int64         `json:"today_events_count"`
	LatestQuotas      []QuotaRecord `json:"latest_quotas"`
}

func NewStore(dbPath string) (*Store, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ingest(env *model.TelemetryEnvelope) error {
	if env == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	nowStr := env.ReportedAt
	if nowStr == "" {
		nowStr = time.Now().UTC().Format(time.RFC3339Nano)
	}

	norm := env.Normalized
	nodeID := env.NodeID
	if nodeID == "" {
		nodeID = "unknown_node"
	}

	// Step 1: UPSERT nodes
	upsertNodeSQL := `
	INSERT INTO nodes (node_id, hostname, os_type, os_arch, ip_address, client_version, first_seen_at, last_seen_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_id) DO UPDATE SET
		hostname = excluded.hostname,
		os_type = excluded.os_type,
		os_arch = excluded.os_arch,
		client_version = excluded.client_version,
		last_seen_at = excluded.last_seen_at;`

	if _, err := tx.Exec(upsertNodeSQL, nodeID, env.Hostname, env.OSType, env.OSArch, "", env.ClientVersion, nowStr, nowStr); err != nil {
		return fmt.Errorf("failed to upsert node: %w", err)
	}

	// Step 2: UPSERT sessions
	sessionID := norm.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%s_%s", nodeID, norm.ProductType)
	}

	upsertSessionSQL := `
	INSERT INTO sessions (
		session_id, node_id, product_type, conversation_id, session_name,
		account_email, plan_tier, tool_version, cwd, project_dir, transcript_path,
		git_repo_host, git_repo_owner, git_repo_name, git_branch, git_worktree,
		latest_model_id, total_input_tokens, total_output_tokens, context_used_pct,
		total_cost_usd, total_duration_ms, total_lines_added, total_lines_removed,
		started_at, last_active_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(session_id) DO UPDATE SET
		node_id = excluded.node_id,
		session_name = CASE WHEN excluded.session_name != '' THEN excluded.session_name ELSE sessions.session_name END,
		account_email = CASE WHEN excluded.account_email != '' THEN excluded.account_email ELSE sessions.account_email END,
		plan_tier = CASE WHEN excluded.plan_tier != '' THEN excluded.plan_tier ELSE sessions.plan_tier END,
		latest_model_id = excluded.latest_model_id,
		total_input_tokens = excluded.total_input_tokens,
		total_output_tokens = excluded.total_output_tokens,
		context_used_pct = excluded.context_used_pct,
		total_cost_usd = excluded.total_cost_usd,
		total_duration_ms = excluded.total_duration_ms,
		total_lines_added = excluded.total_lines_added,
		total_lines_removed = excluded.total_lines_removed,
		last_active_at = excluded.last_active_at;`

	if _, err := tx.Exec(upsertSessionSQL,
		sessionID, nodeID, norm.ProductType, norm.ConversationID, norm.SessionName,
		norm.AccountEmail, norm.PlanTier, norm.ToolVersion, norm.CWD, norm.ProjectDir, norm.TranscriptPath,
		norm.GitRepoHost, norm.GitRepoOwner, norm.GitRepoName, norm.GitBranch, norm.GitWorktree,
		norm.ModelID, norm.TotalInputTokens, norm.TotalOutputTokens, norm.UsedPercentage,
		norm.TotalCostUSD, norm.TotalDurationMS, norm.TotalLinesAdded, norm.TotalLinesRemoved,
		nowStr, nowStr,
	); err != nil {
		return fmt.Errorf("failed to upsert session: %w", err)
	}

	// Step 3: INSERT telemetry_events
	quotaJSON, _ := json.Marshal(norm.Quotas)
	rawPayloadJSON, _ := json.Marshal(env.Payload)
	exceedsInt := 0
	if norm.Exceeds200k {
		exceedsInt = 1
	}

	insertEventSQL := `
	INSERT INTO telemetry_events (
		node_id, session_id, prompt_id, product_type, reported_at,
		model_id, model_display_name, model_effort, agent_state,
		context_window_size, total_input_tokens, total_output_tokens,
		current_input_tokens, current_output_tokens, cache_creation_tokens, cache_read_tokens,
		used_percentage, remaining_percentage, exceeds_200k_tokens,
		total_cost_usd, total_duration_ms, total_api_duration_ms, cache_hit_ratio,
		quota_snapshot, prompt_cache_detail, raw_payload
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.Exec(insertEventSQL,
		nodeID, sessionID, norm.PromptID, norm.ProductType, nowStr,
		norm.ModelID, norm.ModelDisplayName, norm.ModelEffort, norm.AgentState,
		norm.ContextWindowSize, norm.TotalInputTokens, norm.TotalOutputTokens,
		norm.CurrentInputTokens, norm.CurrentOutputTokens, norm.CacheCreationTokens, norm.CacheReadTokens,
		norm.UsedPercentage, norm.RemainingPct, exceedsInt,
		norm.TotalCostUSD, norm.TotalDurationMS, norm.TotalAPIDuration, norm.CacheHitRatio,
		string(quotaJSON), "", string(rawPayloadJSON),
	); err != nil {
		return fmt.Errorf("failed to insert telemetry event: %w", err)
	}

	// Step 4: UPSERT account_quotas
	if norm.AccountEmail != "" && len(norm.Quotas) > 0 {
		upsertQuotaSQL := `
		INSERT INTO account_quotas (account_email, quota_key, remaining_fraction, reset_time, reset_in_seconds, last_node_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_email, quota_key) DO UPDATE SET
			remaining_fraction = excluded.remaining_fraction,
			reset_time = excluded.reset_time,
			reset_in_seconds = excluded.reset_in_seconds,
			last_node_id = excluded.last_node_id,
			updated_at = excluded.updated_at;`

		for qKey, qItem := range norm.Quotas {
			if _, err := tx.Exec(upsertQuotaSQL, norm.AccountEmail, qKey, qItem.RemainingFraction, qItem.ResetTime, qItem.ResetInSeconds, nodeID, nowStr); err != nil {
				return fmt.Errorf("failed to upsert account quota %s: %w", qKey, err)
			}
		}
	}

	return tx.Commit()
}

func (s *Store) GetNodes() ([]NodeRecord, error) {
	rows, err := s.db.Query("SELECT node_id, hostname, os_type, os_arch, COALESCE(ip_address, ''), COALESCE(client_version, ''), first_seen_at, last_seen_at FROM nodes ORDER BY last_seen_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.NodeID, &n.Hostname, &n.OSType, &n.OSArch, &n.IPAddress, &n.ClientVersion, &n.FirstSeenAt, &n.LastSeenAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, nil
}

func (s *Store) GetSessions(nodeID string, limit int) ([]SessionRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
	SELECT session_id, node_id, product_type, COALESCE(conversation_id, ''), COALESCE(session_name, ''),
	       COALESCE(account_email, ''), COALESCE(plan_tier, ''), COALESCE(tool_version, ''),
	       cwd, COALESCE(project_dir, ''), COALESCE(transcript_path, ''),
	       COALESCE(git_repo_host, ''), COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''),
	       COALESCE(git_branch, ''), COALESCE(git_worktree, ''),
	       COALESCE(latest_model_id, ''), total_input_tokens, total_output_tokens, context_used_pct,
	       total_cost_usd, total_duration_ms, total_lines_added, total_lines_removed,
	       started_at, last_active_at
	FROM sessions `

	var args []interface{}
	if nodeID != "" {
		query += "WHERE node_id = ? "
		args = append(args, nodeID)
	}
	query += "ORDER BY last_active_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(
			&r.SessionID, &r.NodeID, &r.ProductType, &r.ConversationID, &r.SessionName,
			&r.AccountEmail, &r.PlanTier, &r.ToolVersion,
			&r.CWD, &r.ProjectDir, &r.TranscriptPath,
			&r.GitRepoHost, &r.GitRepoOwner, &r.GitRepoName,
			&r.GitBranch, &r.GitWorktree,
			&r.LatestModelID, &r.TotalInputTokens, &r.TotalOutputTokens, &r.ContextUsedPct,
			&r.TotalCostUSD, &r.TotalDurationMS, &r.TotalLinesAdded, &r.TotalLinesRemoved,
			&r.StartedAt, &r.LastActiveAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) GetOverview() (*OverviewData, error) {
	ov := &OverviewData{}

	_ = s.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&ov.ActiveNodes)
	_ = s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(total_input_tokens), 0), COALESCE(SUM(total_output_tokens), 0), COALESCE(SUM(total_cost_usd), 0) FROM sessions").
		Scan(&ov.ActiveSessions, &ov.TotalInputTokens, &ov.TotalOutputTokens, &ov.TotalCostUSD)

	ov.TotalTokens = ov.TotalInputTokens + ov.TotalOutputTokens
	_ = s.db.QueryRow("SELECT COUNT(*) FROM telemetry_events WHERE date(reported_at) = date('now')").Scan(&ov.TodayEventsCount)

	quotas, err := s.GetAccountQuotas()
	if err == nil {
		ov.LatestQuotas = quotas
	}

	return ov, nil
}

func (s *Store) GetHistory(sessionID string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
	SELECT id, node_id, session_id, COALESCE(prompt_id, ''), product_type, reported_at,
	       model_id, COALESCE(model_display_name, ''), COALESCE(model_effort, ''), COALESCE(agent_state, ''),
	       COALESCE(context_window_size, 0), COALESCE(total_input_tokens, 0), COALESCE(total_output_tokens, 0),
	       COALESCE(current_input_tokens, 0), COALESCE(current_output_tokens, 0),
	       COALESCE(cache_creation_tokens, 0), COALESCE(cache_read_tokens, 0),
	       COALESCE(used_percentage, 0.0), COALESCE(remaining_percentage, 0.0), exceeds_200k_tokens,
	       COALESCE(total_cost_usd, 0.0), COALESCE(total_duration_ms, 0), COALESCE(total_api_duration_ms, 0),
	       COALESCE(cache_hit_ratio, 0.0), COALESCE(quota_snapshot, '')
	FROM telemetry_events
	WHERE session_id = ?
	ORDER BY reported_at DESC
	LIMIT ?`

	rows, err := s.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []EventRecord
	for rows.Next() {
		var e EventRecord
		var exceedsInt int
		if err := rows.Scan(
			&e.ID, &e.NodeID, &e.SessionID, &e.PromptID, &e.ProductType, &e.ReportedAt,
			&e.ModelID, &e.ModelDisplayName, &e.ModelEffort, &e.AgentState,
			&e.ContextWindowSize, &e.TotalInputTokens, &e.TotalOutputTokens,
			&e.CurrentInputTokens, &e.CurrentOutputTokens,
			&e.CacheCreationTokens, &e.CacheReadTokens,
			&e.UsedPercentage, &e.RemainingPct, &exceedsInt,
			&e.TotalCostUSD, &e.TotalDurationMS, &e.TotalAPIDuration,
			&e.CacheHitRatio, &e.QuotaSnapshot,
		); err != nil {
			return nil, err
		}
		e.Exceeds200k = exceedsInt == 1
		list = append(list, e)
	}
	return list, nil
}

func (s *Store) GetAccountQuotas() ([]QuotaRecord, error) {
	rows, err := s.db.Query("SELECT account_email, quota_key, remaining_fraction, reset_time, reset_in_seconds, COALESCE(last_node_id, ''), updated_at FROM account_quotas ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QuotaRecord
	for rows.Next() {
		var q QuotaRecord
		if err := rows.Scan(&q.AccountEmail, &q.QuotaKey, &q.RemainingFraction, &q.ResetTime, &q.ResetInSeconds, &q.LastNodeID, &q.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, q)
	}
	return list, nil
}
