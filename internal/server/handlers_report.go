package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"statusline/internal/model"
	"statusline/internal/node"
)

func (s *Server) checkAPIKey(r *http.Request) bool {
	if s.cfg == nil || s.cfg.Server.APIKey == "" {
		return true
	}
	key := r.Header.Get("X-API-Key")
	if key == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return key == s.cfg.Server.APIKey
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(r) {
		slog.Warn("report: unauthorized API key", "remote_addr", r.RemoteAddr)
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		slog.Warn("report: empty or unreadable request body", "error", err)
		http.Error(w, `{"error":"empty request body"}`, http.StatusBadRequest)
		return
	}

	// Check if already an envelope
	var env model.TelemetryEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Normalized.ProductType != "" {
		if env.NodeID == "" {
			env.NodeID = "node_default"
		}
		if err := s.store.Ingest(&env); err != nil {
			slog.Error("report: failed to ingest envelope", "node_id", env.NodeID, "session_id", env.Normalized.SessionID, "error", err)
			http.Error(w, `{"error":"failed to store telemetry"}`, http.StatusInternalServerError)
			return
		}
		slog.Info("report: ingested envelope",
			"node_id", env.NodeID,
			"session_id", env.Normalized.SessionID,
			"product", env.Normalized.ProductType,
			"model", env.Normalized.ModelID,
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Otherwise parse and normalize raw payload
	n := node.GetNodeInfo(env.NodeID, "1.0.0")
	parsedEnv, _, err := model.ParseAndNormalize(body, n)
	if err != nil {
		slog.Warn("report: failed to parse raw payload", "error", err)
		http.Error(w, `{"error":"failed to parse telemetry"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.Ingest(parsedEnv); err != nil {
		slog.Error("report: failed to ingest parsed payload", "node_id", parsedEnv.NodeID, "error", err)
		http.Error(w, `{"error":"failed to store telemetry"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("report: ingested raw payload",
		"node_id", parsedEnv.NodeID,
		"session_id", parsedEnv.Normalized.SessionID,
		"product", parsedEnv.Normalized.ProductType,
		"model", parsedEnv.Normalized.ModelID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
