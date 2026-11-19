package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"statusline/internal/config"
	"statusline/internal/node"
	"statusline/internal/storage"
)

// SessionItem wraps SessionRecord with enriched local machine status.
type SessionItem struct {
	Record       storage.SessionRecord `json:"record"`
	IsLocalNode  bool                  `json:"is_local_node"`
	HasValidCWD  bool                  `json:"has_valid_cwd"`
	RelativeTime string                `json:"relative_time"`
	DisplayName  string                `json:"display_name"`
}

// SessionService provides filtered and enriched sessions.
type SessionService interface {
	ListSessions(ctx context.Context, onlyLocalNode bool) ([]SessionItem, error)
	GetLocalNodeID() string
}

// SourceMode defines how sessions are fetched.
type SourceMode string

const (
	SourceAuto   SourceMode = "auto"   // Prefer remote API if configured/available, fall back to local DB
	SourceRemote SourceMode = "remote" // Strictly fetch from remote API
	SourceDB     SourceMode = "db"     // Strictly fetch from local SQLite DB
)

type DefaultSessionService struct {
	store       *storage.Store
	cfg         *config.Config
	localNodeID string
	sourceMode  SourceMode
	mockRecords []storage.SessionRecord
}

// NewSessionService creates a SessionService backed by SQLite and/or HTTP remote API.
func NewSessionService(store *storage.Store, cfg *config.Config) *DefaultSessionService {
	customNodeID := ""
	if cfg != nil {
		customNodeID = cfg.Node.ID
	}
	n := node.GetNodeInfo(customNodeID, "session-tui")
	return &DefaultSessionService{
		store:       store,
		cfg:         cfg,
		localNodeID: n.NodeID,
		sourceMode:  SourceAuto,
	}
}

// SetSourceMode updates the source mode (auto, remote, db).
func (s *DefaultSessionService) SetSourceMode(mode SourceMode) {
	s.sourceMode = mode
}

// NewSessionServiceWithRecords creates an in-memory test SessionService.
func NewSessionServiceWithRecords(localNodeID string, records []storage.SessionRecord) *DefaultSessionService {
	return &DefaultSessionService{
		localNodeID: localNodeID,
		mockRecords: records,
		sourceMode:  SourceAuto,
	}
}

func (s *DefaultSessionService) GetLocalNodeID() string {
	return s.localNodeID
}

type remoteSessionDTO struct {
	ProductType    string `json:"product_type"`
	SessionID      string `json:"session_id"`
	NodeID         string `json:"node_id"`
	ConversationID string `json:"conversation_id"`
	SessionName    string `json:"session_name"`
	Timestamp      int64  `json:"timestamp"`
	CWD            string `json:"cwd"`
	ProjectDir     string `json:"project_dir"`
	TranscriptPath string `json:"transcript_path"`
	GitBranch      string `json:"git_branch"`
	Model          struct {
		ID          string `json:"model_id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Account struct {
		Email    string `json:"email"`
		PlanTier string `json:"plan_tier"`
	} `json:"account"`
	Tokens struct {
		TotalInputTokens     int64 `json:"total_input_tokens"`
		TotalOutputTokens    int64 `json:"total_output_tokens"`
		TotalCacheReadTokens int64 `json:"total_cache_read_tokens"`
		TotalTokens          int64 `json:"total_tokens"`
	} `json:"tokens"`
	ContextWindow struct {
		UsedPercentage float64 `json:"used_pct"`
	} `json:"context_window"`
}

func (s *DefaultSessionService) fetchRemoteSessions(ctx context.Context, onlyLocalNode bool) ([]storage.SessionRecord, error) {
	if s.cfg == nil || s.cfg.Report.Endpoint == "" || s.cfg.Report.Endpoint == "off" || s.cfg.Report.Endpoint == "none" {
		return nil, fmt.Errorf("未配置有效的主机上报地址 (report.endpoint)")
	}
	endpoint := s.cfg.Report.Endpoint
	base := endpoint
	if idx := strings.Index(base, "/api/"); idx != -1 {
		base = base[:idx]
	}
	sessionsURL := base + "/api/v1/sessions?limit=200"
	if onlyLocalNode && s.localNodeID != "" {
		sessionsURL += "&node_id=" + s.localNodeID
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionsURL, nil)
	if err != nil {
		return nil, err
	}
	if s.cfg.Report.APIKey != "" {
		req.Header.Set("X-API-Key", s.cfg.Report.APIKey)
		req.Header.Set("Authorization", "Bearer "+s.cfg.Report.APIKey)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("远端 HTTP 状态码异常: %d", resp.StatusCode)
	}

	var dtos []remoteSessionDTO
	if err := json.NewDecoder(resp.Body).Decode(&dtos); err != nil {
		return nil, err
	}

	var records []storage.SessionRecord
	for _, dto := range dtos {
		timeStr := ""
		if dto.Timestamp > 0 {
			timeStr = time.Unix(dto.Timestamp, 0).Format(time.RFC3339)
		}
		modelName := dto.Model.DisplayName
		if modelName == "" {
			modelName = dto.Model.ID
		}
		rec := storage.SessionRecord{
			SessionID:            dto.SessionID,
			NodeID:               dto.NodeID,
			ProductType:          dto.ProductType,
			ConversationID:       dto.ConversationID,
			SessionName:          dto.SessionName,
			AccountEmail:         dto.Account.Email,
			PlanTier:             dto.Account.PlanTier,
			CWD:                  dto.CWD,
			ProjectDir:           dto.ProjectDir,
			TranscriptPath:       dto.TranscriptPath,
			GitBranch:            dto.GitBranch,
			LatestModelID:        modelName,
			TotalInputTokens:     dto.Tokens.TotalInputTokens,
			TotalOutputTokens:    dto.Tokens.TotalOutputTokens,
			TotalCacheReadTokens: dto.Tokens.TotalCacheReadTokens,
			TotalTokens:          dto.Tokens.TotalTokens,
			ContextUsedPct:       dto.ContextWindow.UsedPercentage,
			LastActiveAt:         timeStr,
		}
		records = append(records, rec)
	}
	return records, nil
}

func (s *DefaultSessionService) ListSessions(ctx context.Context, onlyLocalNode bool) ([]SessionItem, error) {
	var rawRecords []storage.SessionRecord

	if s.mockRecords != nil {
		rawRecords = s.mockRecords
	} else {
		mode := s.sourceMode
		if mode == "" {
			mode = SourceAuto
		}

		switch mode {
		case SourceRemote:
			recs, err := s.fetchRemoteSessions(ctx, onlyLocalNode)
			if err != nil {
				return nil, fmt.Errorf("远端接口获取会话失败: %w", err)
			}
			rawRecords = recs

		case SourceDB:
			if s.store == nil {
				return nil, fmt.Errorf("本地 SQLite 数据库未打开")
			}
			nodeFilter := ""
			if onlyLocalNode {
				nodeFilter = s.localNodeID
			}
			recs, err := s.store.GetSessions(nodeFilter, 200)
			if err != nil {
				return nil, fmt.Errorf("读取本地 SQLite 数据库失败: %w", err)
			}
			rawRecords = recs

		default: // SourceAuto
			// 1. If remote endpoint configured and not off, try remote first
			if s.cfg != nil && s.cfg.Report.Endpoint != "" && s.cfg.Report.Endpoint != "off" && s.cfg.Report.Endpoint != "none" {
				recs, err := s.fetchRemoteSessions(ctx, onlyLocalNode)
				if err == nil {
					rawRecords = recs
				}
			}

			// 2. If remote returned empty or failed, fall back to local DB
			if len(rawRecords) == 0 && s.store != nil {
				nodeFilter := ""
				if onlyLocalNode {
					nodeFilter = s.localNodeID
				}
				recs, err := s.store.GetSessions(nodeFilter, 200)
				if err == nil && len(recs) > 0 {
					rawRecords = recs
				}
			}
		}
	}

	// Sort rawRecords by LastActiveAt descending
	sort.Slice(rawRecords, func(i, j int) bool {
		return rawRecords[i].LastActiveAt > rawRecords[j].LastActiveAt
	})

	var items []SessionItem
	for _, rec := range rawRecords {
		isLocal := rec.NodeID == s.localNodeID || rec.NodeID == ""

		if onlyLocalNode && !isLocal {
			continue
		}

		// Check CWD existence on this machine
		hasCWD := false
		if rec.CWD != "" {
			if fi, err := os.Stat(rec.CWD); err == nil && fi.IsDir() {
				hasCWD = true
			}
		}

		// Calculate display name
		displayName := rec.GitRepoName
		if displayName == "" {
			if rec.CWD != "" {
				displayName = filepath.Base(rec.CWD)
			} else {
				displayName = rec.SessionID
			}
		}

		items = append(items, SessionItem{
			Record:       rec,
			IsLocalNode:  isLocal,
			HasValidCWD:  hasCWD,
			RelativeTime: FormatRelativeTime(rec.LastActiveAt),
			DisplayName:  displayName,
		})
	}

	return items, nil
}

// FormatRelativeTime converts an RFC3339 timestamp into compact friendly relative time.
func FormatRelativeTime(timeStr string) string {
	if timeStr == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", timeStr)
		if err != nil {
			return timeStr
		}
	}

	now := time.Now()
	diff := now.Sub(t)
	if diff < 0 {
		diff = 0
	}

	secs := int64(diff.Seconds())
	if secs < 60 {
		return "刚刚"
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm 前", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%dh 前", hours)
	}
	days := hours / 24
	if days == 1 {
		return "昨天"
	}
	if days < 7 {
		return fmt.Sprintf("%dd 前", days)
	}
	return t.Format("01-02")
}
