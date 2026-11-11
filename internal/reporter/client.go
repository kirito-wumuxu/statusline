package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
		return
	}

	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// ReportAsync dispatches telemetry reporting in a background detached goroutine.
func ReportAsync(env *model.TelemetryEnvelope, serverURL string, timeout time.Duration) {
	go Report(env, serverURL, timeout)
}
