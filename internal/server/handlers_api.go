package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"statusline/internal/model"
)

func (s *Server) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	sessions, err := s.store.GetSessions(nodeID, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to get sessions"}`, http.StatusInternalServerError)
		return
	}

	// Format sessions for API / Web dashboard compatibility
	type sessionDTO struct {
		Source            string               `json:"source"`
		ProductType       string               `json:"product_type"`
		SessionID         string               `json:"session_id"`
		NodeID            string               `json:"node_id"`
		ConversationID    string               `json:"conversation_id,omitempty"`
		ConversationTitle string               `json:"conversation_title,omitempty"`
		SessionName       string               `json:"session_name,omitempty"`
		Timestamp         int64                `json:"timestamp"`
		CWD               string               `json:"cwd"`
		Model             model.ModelSummary   `json:"model"`
		Account           model.AccountSummary `json:"account"`
		Tokens            model.TokensSummary  `json:"tokens"`
		ContextWindow     model.ContextSummary `json:"context_window"`
		CostUSD           float64              `json:"cost_usd"`
		AgentState        string               `json:"agent_state"`
	}

	var dtoList []sessionDTO
	for _, sess := range sessions {
		var ts int64
		if t, err := time.Parse(time.RFC3339Nano, sess.LastActiveAt); err == nil {
			ts = t.Unix()
		} else {
			ts = time.Now().Unix()
		}

		source := sess.ProductType
		if source == "claude_code" {
			source = "claudecode"
		}

		dto := sessionDTO{
			Source:            source,
			ProductType:       sess.ProductType,
			SessionID:         sess.SessionID,
			NodeID:            sess.NodeID,
			ConversationID:    sess.ConversationID,
			ConversationTitle: sess.SessionName,
			SessionName:       sess.SessionName,
			Timestamp:         ts,
			CWD:               sess.CWD,
			Model: model.ModelSummary{
				ID:          sess.LatestModelID,
				DisplayName: sess.LatestModelID,
			},
			Account: model.AccountSummary{
				Email:    sess.AccountEmail,
				PlanTier: sess.PlanTier,
			},
			Tokens: model.TokensSummary{
				TotalInputTokens:  sess.TotalInputTokens,
				TotalOutputTokens: sess.TotalOutputTokens,
				TotalTokens:       sess.TotalInputTokens + sess.TotalOutputTokens,
			},
			ContextWindow: model.ContextSummary{
				ContextWindowSize: 1048576,
				UsedPercentage:    sess.ContextUsedPct,
			},
			CostUSD:    sess.TotalCostUSD,
			AgentState: "idle",
		}
		dtoList = append(dtoList, dto)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dtoList)
}

func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.GetNodes()
	if err != nil {
		http.Error(w, `{"error":"failed to get nodes"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(nodes)
}

func (s *Server) handleGetOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.store.GetOverview()
	if err != nil {
		http.Error(w, `{"error":"failed to get overview"}`, http.StatusInternalServerError)
		return
	}

	sessions, _ := s.store.GetSessions("", 100)
	sourceBreakdown := make(map[string]int)
	modelBreakdown := make(map[string]int)

	var latestAccount model.AccountSummary
	for _, sess := range sessions {
		src := sess.ProductType
		if src == "claude_code" {
			src = "claudecode"
		}
		sourceBreakdown[src]++
		if sess.LatestModelID != "" {
			modelBreakdown[sess.LatestModelID]++
		}
		if sess.AccountEmail != "" && latestAccount.Email == "" {
			latestAccount.Email = sess.AccountEmail
			latestAccount.PlanTier = sess.PlanTier
		}
	}

	// Build latest quota summary
	quotaSummary := model.QuotaSummary{
		Gemini: model.ProviderQuota{ShortPercentage: 100, ShortReset: "2h0m", LongPercentage: 100, LongReset: "7d0h"},
		Claude: model.ProviderQuota{ShortPercentage: 100, ShortReset: "5h0m", LongPercentage: 100, LongReset: "7d0h"},
	}

	for _, q := range ov.LatestQuotas {
		lower := strings.ToLower(q.QuotaKey)
		pct := int(q.RemainingFraction * 100)
		dur := model.FormatDurationCompact(q.ResetInSeconds)
		if strings.Contains(lower, "gemini") {
			quotaSummary.Gemini.ShortPercentage = pct
			quotaSummary.Gemini.ShortReset = dur
		} else if strings.Contains(lower, "claude") || strings.Contains(lower, "3p") {
			quotaSummary.Claude.ShortPercentage = pct
			quotaSummary.Claude.ShortReset = dur
		}
	}

	res := map[string]interface{}{
		"active_sessions":          ov.ActiveSessions,
		"active_sessions_count":    ov.ActiveSessions,
		"active_nodes":             ov.ActiveNodes,
		"active_nodes_count":       ov.ActiveNodes,
		"total_input_tokens":       ov.TotalInputTokens,
		"total_output_tokens":      ov.TotalOutputTokens,
		"total_tokens":             ov.TotalTokens,
		"total_cost_usd":           ov.TotalCostUSD,
		"today_events_count":       ov.TodayEventsCount,
		"source_breakdown":         sourceBreakdown,
		"model_breakdown":          modelBreakdown,
		"latest_account":           latestAccount,
		"latest_quota":             quotaSummary,
		"refresh_interval_seconds": 60,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history, err := s.store.GetHistory(sessionID, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to get history"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(history)
}

func (s *Server) handleGetQuotas(w http.ResponseWriter, r *http.Request) {
	quotas, err := s.store.GetAccountQuotas()
	if err != nil {
		http.Error(w, `{"error":"failed to get quotas"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(quotas)
}
