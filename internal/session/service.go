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

type DefaultSessionService struct {
	store       *storage.Store
	cfg         *config.Config
	localNodeID string
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
	}
}

// NewSessionServiceWithRecords creates an in-memory test SessionService.
func NewSessionServiceWithRecords(localNodeID string, records []storage.SessionRecord) *DefaultSessionService {
	return &DefaultSessionService{
		localNodeID: localNodeID,
		mockRecords: records,
	}
}

func (s *DefaultSessionService) GetLocalNodeID() string {
	return s.localNodeID
}

func (s *DefaultSessionService) ListSessions(ctx context.Context, onlyLocalNode bool) ([]SessionItem, error) {
	var rawRecords []storage.SessionRecord

	if s.mockRecords != nil {
		rawRecords = s.mockRecords
	} else if s.store != nil {
		nodeFilter := ""
		if onlyLocalNode {
			nodeFilter = s.localNodeID
		}
		recs, err := s.store.GetSessions(nodeFilter, 200)
		if err == nil && len(recs) > 0 {
			rawRecords = recs
		}
	}

	// If no local DB records, attempt fetching from remote statusline-server if configured
	if len(rawRecords) == 0 && s.cfg != nil && s.cfg.Report.Endpoint != "" && s.cfg.Report.Endpoint != "off" && s.cfg.Report.Endpoint != "none" {
		endpoint := s.cfg.Report.Endpoint
		base := endpoint
		if idx := strings.Index(base, "/api/"); idx != -1 {
			base = base[:idx]
		}
		sessionsURL := base + "/api/v1/sessions"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionsURL, nil)
		if err == nil {
			if s.cfg.Report.APIKey != "" {
				req.Header.Set("X-API-Key", s.cfg.Report.APIKey)
				req.Header.Set("Authorization", "Bearer "+s.cfg.Report.APIKey)
			}
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				_ = json.NewDecoder(resp.Body).Decode(&rawRecords)
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
