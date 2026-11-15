package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// GitHubSource downloads releases from GitHub Releases API.
type GitHubSource struct {
	Owner      string
	Repo       string
	HTTPClient *http.Client
}

// NewGitHubSource creates a new GitHubSource.
func NewGitHubSource(owner, repo string) *GitHubSource {
	return &GitHubSource{
		Owner: owner,
		Repo:  repo,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (g *GitHubSource) Name() string {
	return "github"
}

type gitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type gitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	Assets      []gitHubAsset `json:"assets"`
}

func (g *GitHubSource) CheckRelease(ctx context.Context, targetVersion string) (*ReleaseInfo, error) {
	var apiURL string
	if targetVersion == "" || targetVersion == "latest" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", g.Owner, g.Repo)
	} else {
		tag := targetVersion
		if !strings.HasPrefix(tag, "v") && !strings.Contains(tag, ".") {
			tag = "v" + tag
		}
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", g.Owner, g.Repo, tag)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "statusline-updater")

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var ghRel gitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRel); err != nil {
		return nil, fmt.Errorf("invalid json from GitHub API: %w", err)
	}

	assetsMap := make(map[string]string)
	var checksumsURL string

	for _, a := range ghRel.Assets {
		name := strings.ToLower(a.Name)
		if strings.HasPrefix(name, "checksums") && strings.HasSuffix(name, ".txt") {
			checksumsURL = a.BrowserDownloadURL
			continue
		}
		// Match platform key from asset name, e.g. statusline_v0.1.6_linux_amd64.tar.gz
		for _, osName := range []string{"linux", "darwin", "windows"} {
			for _, archName := range []string{"amd64", "arm64"} {
				pattern := fmt.Sprintf("_%s_%s", osName, archName)
				if strings.Contains(name, pattern) {
					key := fmt.Sprintf("%s_%s", osName, archName)
					assetsMap[key] = a.BrowserDownloadURL
				}
			}
		}
	}

	checksums := make(map[string]string)
	if checksumsURL != "" {
		if cs, err := g.fetchChecksums(ctx, checksumsURL); err == nil {
			checksums = cs
		}
	}

	return &ReleaseInfo{
		Version:      ghRel.TagName,
		TagName:      ghRel.TagName,
		ReleaseNotes: ghRel.Body,
		PublishedAt:  ghRel.PublishedAt,
		Assets:       assetsMap,
		Checksums:    checksums,
		Source:       "github",
	}, nil
}

func (g *GitHubSource) fetchChecksums(ctx context.Context, checksumsURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", checksumsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "statusline-updater")
	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := parts[0]
			fname := filepath.Base(parts[1])
			res[fname] = hash
		}
	}
	return res, nil
}

func (g *GitHubSource) Download(ctx context.Context, assetURL, destPath string, progressFn func(downloaded, total int64)) error {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "statusline-updater")
	return downloadWithProgress(ctx, g.HTTPClient, req, destPath, progressFn)
}
