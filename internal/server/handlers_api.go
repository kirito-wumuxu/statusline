package server

import (
	"encoding/json"
	"io"
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

	metrics, _ := s.store.GetRecentMetrics(50)
	groupedQuotas, _ := s.store.GetGroupedAccountQuotas()
	dailyTokens, _ := s.store.GetDailyModelTokens(7)

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
		"account_quotas":           groupedQuotas,
		"daily_tokens":             dailyTokens,
		"metrics":                  metrics,
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
	quotas, err := s.store.GetGroupedAccountQuotas()
	if err != nil {
		http.Error(w, `{"error":"failed to get quotas"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(quotas)
}

func (s *Server) handleGetDailyTokens(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 14
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	dailyTokens, err := s.store.GetDailyModelTokens(days)
	if err != nil {
		http.Error(w, `{"error":"failed to get daily tokens"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dailyTokens)
}

func (s *Server) handleGetDebugRaw(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 5
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	rawReports, err := s.store.GetRecentRawReports(limit)
	if err != nil {
		http.Error(w, `{"error":"failed to get raw reports"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rawReports)
}


func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","refreshed_count":0}`))
}

func (s *Server) handleGetAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.store.GetDisplayAliases()
	if err != nil {
		http.Error(w, `{"error":"failed to get aliases"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aliases)
}

func (s *Server) handleSaveAliases(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var items []model.DisplayAliasItem
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, `{"error":"empty body"}`, http.StatusBadRequest)
		return
	}

	// Supports both single item or slice or grouped object
	if err := json.Unmarshal(body, &items); err != nil {
		var single model.DisplayAliasItem
		if err2 := json.Unmarshal(body, &single); err2 == nil && single.Category != "" && single.Key != "" {
			items = []model.DisplayAliasItem{single}
		} else {
			var grp model.DisplayAliasesConfig
			if err3 := json.Unmarshal(body, &grp); err3 == nil {
				for _, q := range grp.Quotas {
					q.Category = "quota"
					items = append(items, q)
				}
				for _, m := range grp.Models {
					m.Category = "model"
					items = append(items, m)
				}
				for _, a := range grp.Accounts {
					a.Category = "account"
					items = append(items, a)
				}
			} else {
				http.Error(w, `{"error":"invalid json format"}`, http.StatusBadRequest)
				return
			}
		}
	}

	if err := s.store.SaveDisplayAliases(items); err != nil {
		http.Error(w, `{"error":"failed to save aliases"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cat := r.URL.Query().Get("category")
	key := r.URL.Query().Get("key")
	if cat == "" || key == "" {
		http.Error(w, `{"error":"category and key required"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteDisplayAlias(cat, key); err != nil {
		http.Error(w, `{"error":"failed to delete alias"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
