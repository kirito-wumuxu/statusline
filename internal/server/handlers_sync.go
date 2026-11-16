package server

import (
	"encoding/json"
	"net/http"
	"time"

	"statusline/internal/storage"
)

func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	var batch storage.SyncBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	start := time.Now()
	res, err := s.store.IngestSyncBatch(r.Context(), &batch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"status":     "ok",
		"received":   res.Received,
		"inserted":   res.Inserted,
		"skipped":    res.Skipped,
		"elapsed_ms": time.Since(start).Milliseconds(),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
