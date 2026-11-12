package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"statusline/internal/model"
)

// Report sends a telemetry envelope to the server synchronously with strict timeout.
// Errors and timeouts are completely discarded to ensure terminal non-blocking.
func Report(env *model.TelemetryEnvelope, serverURL string, timeout time.Duration) {
	if env == nil || serverURL == "" || serverURL == "off" || serverURL == "none" {
		return
	}

	payload, err := json.Marshal(env)
	if err != nil {
		slog.Warn("failed to marshal telemetry envelope", "error", err)
		return
	}

	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	slog.Debug("reporting telemetry", "url", serverURL, "payload_bytes", len(payload), "timeout_ms", timeout.Milliseconds())

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(payload))
	if err != nil {
		slog.Warn("failed to create report request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("telemetry report failed", "url", serverURL, "error", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	slog.Debug("telemetry reported", "status", resp.StatusCode)
}

// ReportAsync dispatches telemetry reporting in a background detached goroutine.
func ReportAsync(env *model.TelemetryEnvelope, serverURL string, timeout time.Duration) {
	go Report(env, serverURL, timeout)
}
