package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"statusline/internal/updater"
)

func (s *Server) handleUploadReleases(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cleanDir := resolveReleasesDir(s.cfg.Storage.ReleasesDir)
	_ = os.MkdirAll(cleanDir, 0755)

	// Max 256MB for multipart uploads
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse multipart form: %v"}`, err), http.StatusBadRequest)
		return
	}

	var savedFiles []string
	form := r.MultipartForm
	if form != nil && form.File != nil {
		for _, fileHeaders := range form.File {
			for _, fh := range fileHeaders {
				src, err := fh.Open()
				if err != nil {
					continue
				}

				safeName := filepath.Base(fh.Filename)
				destPath := filepath.Join(cleanDir, safeName)
				dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					src.Close()
					continue
				}

				_, _ = io.Copy(dst, src)
				dst.Close()
				src.Close()
				savedFiles = append(savedFiles, safeName)
			}
		}
	}

	catalog := scanReleasesDir(s.cfg.Storage.ReleasesDir)
	res := map[string]interface{}{
		"status":      "ok",
		"saved_files": savedFiles,
		"catalog":     catalog,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleSyncGitHub(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cleanDir := resolveReleasesDir(s.cfg.Storage.ReleasesDir)
	_ = os.MkdirAll(cleanDir, 0755)

	gh := updater.NewGitHubSource("kirito-wumuxu", "statusline")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	rel, err := gh.CheckRelease(ctx, "latest")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to query github release: %v"}`, err), http.StatusBadGateway)
		return
	}

	var downloaded []string
	for _, assetURL := range rel.Assets {
		filename := filepath.Base(assetURL)
		destPath := filepath.Join(cleanDir, filename)
		if dlErr := gh.Download(ctx, assetURL, destPath, nil); dlErr == nil {
			downloaded = append(downloaded, filename)
		}
	}

	// Also download checksums.txt if available
	if len(rel.Checksums) > 0 {
		csPath := filepath.Join(cleanDir, "checksums.txt")
		f, cErr := os.Create(csPath)
		if cErr == nil {
			for fname, hash := range rel.Checksums {
				_, _ = fmt.Fprintf(f, "%s  %s\n", hash, fname)
			}
			f.Close()
			downloaded = append(downloaded, "checksums.txt")
		}
	}

	catalog := scanReleasesDir(s.cfg.Storage.ReleasesDir)
	res := map[string]interface{}{
		"status":           "ok",
		"synced_version":   rel.Version,
		"downloaded_files": downloaded,
		"catalog":          catalog,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
