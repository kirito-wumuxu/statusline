package reporter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"statusline/internal/model"
)

func TestReportAsyncServerOffline(t *testing.T) {
	// Review Focus #2: Server offline must fail silently in <150ms without error
	start := time.Now()
	env := &model.TelemetryEnvelope{
		NodeID: "node_test",
	}

	// Deliberately unreachable local port
	unreachableURL := "http://127.0.0.1:54321/api/v1/report"
	Report(env, unreachableURL, 50*time.Millisecond)

	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected report to timeout quickly, took %v", elapsed)
	}
}

func TestReportAsyncSuccess(t *testing.T) {
	received := make(chan bool, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var env model.TelemetryEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err == nil {
			if env.NodeID == "node_test_success" {
				received <- true
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	env := &model.TelemetryEnvelope{
		NodeID: "node_test_success",
	}
	ReportAsync(env, ts.URL, 500*time.Millisecond)

	select {
	case <-received:
		// success
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for async report")
	}
}
