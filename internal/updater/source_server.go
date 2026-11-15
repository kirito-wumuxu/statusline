package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ServerSource downloads updates from a self-hosted statusline-server.
type ServerSource struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewServerSource creates a new ServerSource.
func NewServerSource(baseURL, apiKey string) *ServerSource {
	return &ServerSource{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (s *ServerSource) Name() string {
	return "server"
}

type serverVersionResponse struct {
	Status           string            `json:"status"`
	LatestVersion    string            `json:"latest_version"`
	UpgradeSupported bool              `json:"upgrade_supported"`
	ReleaseNotes     string            `json:"release_notes"`
	PublishedAt      string            `json:"published_at"`
	Assets           map[string]string `json:"assets"`
	Checksums        map[string]string `json:"checksums"`
}

func (s *ServerSource) CheckRelease(ctx context.Context, targetVersion string) (*ReleaseInfo, error) {
	reqURL := fmt.Sprintf("%s/api/v1/version", strings.TrimRight(s.BaseURL, "/"))
	if targetVersion != "" && targetVersion != "latest" {
		reqURL += "?version=" + targetVersion
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if s.APIKey != "" {
		req.Header.Set("X-API-Key", s.APIKey)
	}

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query statusline-server version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statusline-server returned HTTP %d", resp.StatusCode)
	}

	var data serverVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid json response from server: %w", err)
	}

	if !data.UpgradeSupported && len(data.Assets) == 0 {
		return nil, fmt.Errorf("statusline-server does not support upgrades or has no hosted releases")
	}

	// Make sure asset URLs are absolute if server returned relative paths
	resolvedAssets := make(map[string]string)
	for k, u := range data.Assets {
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			resolvedAssets[k] = u
		} else {
			resolvedAssets[k] = strings.TrimRight(s.BaseURL, "/") + "/" + strings.TrimLeft(u, "/")
		}
	}

	return &ReleaseInfo{
		Version:      data.LatestVersion,
		TagName:      data.LatestVersion,
		ReleaseNotes: data.ReleaseNotes,
		PublishedAt:  data.PublishedAt,
		Assets:       resolvedAssets,
		Checksums:    data.Checksums,
		Source:       "server",
	}, nil
}

func (s *ServerSource) Download(ctx context.Context, assetURL, destPath string, progressFn func(downloaded, total int64)) error {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return err
	}
	if s.APIKey != "" {
		req.Header.Set("X-API-Key", s.APIKey)
	}
	return downloadWithProgress(ctx, s.HTTPClient, req, destPath, progressFn)
}
