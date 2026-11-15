package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"statusline/internal/config"
)

// CurrentPlatform returns the os_arch key (e.g. "linux_amd64", "windows_amd64", "darwin_arm64").
func CurrentPlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// ReleaseInfo holds metadata about an available release.
type ReleaseInfo struct {
	Version      string            `json:"version"`       // e.g. "v0.1.6"
	TagName      string            `json:"tag_name"`      // e.g. "v0.1.6"
	ReleaseNotes string            `json:"release_notes"` // Release description
	PublishedAt  string            `json:"published_at"`
	Assets       map[string]string `json:"assets"`    // platformKey -> downloadURL
	Checksums    map[string]string `json:"checksums"` // filename -> sha256_hex
	Source       string            `json:"source"`    // "server" or "github"
}

// Source defines the interface for checking and downloading updates.
type Source interface {
	Name() string
	CheckRelease(ctx context.Context, targetVersion string) (*ReleaseInfo, error)
	Download(ctx context.Context, assetURL, destPath string, progressFn func(downloaded, total int64)) error
}

// ResolveSource selects the best available update source:
// If forcedSource is "server" or "github", it honors that choice.
// If forcedSource is "auto" (default), it tests the configured statusline-server.
// If statusline-server is reachable and reports upgrade_supported=true, ServerSource is returned;
// otherwise it falls back to GitHubSource.
func ResolveSource(cfg *config.Config, forcedSource string) (Source, error) {
	forced := strings.ToLower(strings.TrimSpace(forcedSource))
	if forced == "github" {
		return NewGitHubSource("kirito-wumuxu", "statusline"), nil
	}

	serverEndpoint := ""
	apiKey := ""
	if cfg != nil {
		serverEndpoint = cfg.Report.Endpoint
		apiKey = cfg.Report.APIKey
	}

	serverBase := extractServerBaseURL(serverEndpoint)

	if forced == "server" {
		if serverBase == "" {
			return nil, fmt.Errorf("cannot use server update source: report.endpoint is not configured")
		}
		return NewServerSource(serverBase, apiKey), nil
	}

	// Auto mode: probe server if configured and not disabled
	if serverBase != "" && serverBase != "off" && serverBase != "none" {
		serverSrc := NewServerSource(serverBase, apiKey)
		probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		rel, err := serverSrc.CheckRelease(probeCtx, "latest")
		if err == nil && rel != nil && len(rel.Assets) > 0 {
			return serverSrc, nil
		}
	}

	// Fallback to GitHub Releases
	return NewGitHubSource("kirito-wumuxu", "statusline"), nil
}

func extractServerBaseURL(endpoint string) string {
	ep := strings.TrimSpace(endpoint)
	if ep == "" || ep == "off" || ep == "none" {
		return ""
	}
	u, err := url.Parse(ep)
	if err != nil {
		return ""
	}

	// If endpoint is like "http://127.0.0.1:8628/api/v1/report"
	// base is "http://127.0.0.1:8628" (or with subpath like "http://example.com/statusline")
	idx := strings.Index(u.Path, "/api/v1/report")
	if idx != -1 {
		u.Path = u.Path[:idx]
	} else if strings.HasSuffix(u.Path, "/report") {
		u.Path = strings.TrimSuffix(u.Path, "/report")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

// downloadWithProgress is a helper for downloading an HTTP URL to destPath with progress reporting.
func downloadWithProgress(ctx context.Context, client *http.Client, req *http.Request, destPath string, progressFn func(d, total int64)) error {
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	total := resp.ContentLength
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("failed writing downloaded data: %w", wErr)
			}
			downloaded += int64(n)
			if progressFn != nil {
				progressFn(downloaded, total)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return fmt.Errorf("failed reading download stream: %w", rErr)
		}
	}

	return nil
}
