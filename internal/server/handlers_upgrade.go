package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	catalog := scanReleasesDir(s.cfg.Storage.ReleasesDir)

	targetVer := r.URL.Query().Get("version")
	if targetVer != "" && targetVer != "latest" {
		if pkgs, ok := catalog.Versions[targetVer]; ok {
			catalog.LatestVersion = targetVer
			catalog.Assets = make(map[string]string)
			for _, pkg := range pkgs {
				catalog.Assets[pkg.Platform] = pkg.DownloadPath
			}
		}
	}

	serverVer := s.version
	if serverVer == "" {
		serverVer = "dev"
	}

	res := map[string]interface{}{
		"status":            "ok",
		"server_version":    serverVer,
		"latest_version":    catalog.LatestVersion,
		"upgrade_supported": catalog.UpgradeSupported,
		"assets":            catalog.Assets,
		"checksums":         catalog.Checksums,
		"packages":          catalog.Packages,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleDownloadUpgrade(w http.ResponseWriter, r *http.Request) {
	cleanDir := resolveReleasesDir(s.cfg.Storage.ReleasesDir)
	filename := r.URL.Query().Get("filename")
	platform := r.URL.Query().Get("platform")

	if filename == "" && platform != "" {
		catalog := scanReleasesDir(s.cfg.Storage.ReleasesDir)
		for _, pkg := range catalog.Packages {
			if pkg.Platform == platform {
				// Default to matching package from latest version
				if catalog.LatestVersion != "" && pkg.Version == catalog.LatestVersion {
					filename = pkg.Filename
					break
				}
				if filename == "" {
					filename = pkg.Filename
				}
			}
		}
	}

	if filename == "" {
		http.Error(w, `{"error":"filename or platform parameter required"}`, http.StatusBadRequest)
		return
	}

	// Security: prevent directory traversal
	cleanFilename := filepath.Base(filename)
	filePath := filepath.Join(cleanDir, cleanFilename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, `{"error":"release package not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+cleanFilename)
	if strings.HasSuffix(cleanFilename, ".tar.gz") || strings.HasSuffix(cleanFilename, ".tgz") {
		w.Header().Set("Content-Type", "application/gzip")
	} else if strings.HasSuffix(cleanFilename, ".zip") {
		w.Header().Set("Content-Type", "application/zip")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	http.ServeFile(w, r, filePath)
}
