package server

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleSyncTranscripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	scanner := NewTranscriptScanner(s.store)
	start := time.Now()
	res, err := scanner.SyncAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res.ElapsedMS = time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}
