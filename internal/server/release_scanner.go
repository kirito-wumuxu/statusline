package server

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"statusline/internal/updater"
)

type ReleasePackage struct {
	Filename     string `json:"filename"`
	Version      string `json:"version"`
	Platform     string `json:"platform"` // e.g. "linux_amd64"
	Size         int64  `json:"size"`
	Checksum     string `json:"checksum,omitempty"`
	DownloadPath string `json:"download_path"`
}

type ReleaseCatalog struct {
	LatestVersion    string                    `json:"latest_version"`
	UpgradeSupported bool                      `json:"upgrade_supported"`
	Assets           map[string]string         `json:"assets"`    // platform -> download_path
	Checksums        map[string]string         `json:"checksums"` // filename -> sha256
	Packages         []ReleasePackage          `json:"packages"`
	Versions         map[string][]ReleasePackage `json:"versions"`
}

// resolveReleasesDir expands leading ~ and returns clean directory.
func resolveReleasesDir(dir string) string {
	if dir == "" {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".local", "share", "statusline", "releases")
	}
	if strings.HasPrefix(dir, "~/") || dir == "~" {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, dir[1:])
	}
	return filepath.Clean(dir)
}

func scanReleasesDir(releasesDir string) ReleaseCatalog {
	cleanDir := resolveReleasesDir(releasesDir)
	catalog := ReleaseCatalog{
		Assets:    make(map[string]string),
		Checksums: make(map[string]string),
		Versions:  make(map[string][]ReleasePackage),
		Packages:  make([]ReleasePackage, 0),
	}

	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return catalog
	}

	// 1. First pass: read checksums.txt if present
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(strings.ToLower(entry.Name()), "checksums") {
			csPath := filepath.Join(cleanDir, entry.Name())
			if f, err := os.Open(csPath); err == nil {
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						catalog.Checksums[filepath.Base(parts[1])] = parts[0]
					}
				}
				f.Close()
			}
		}
	}

	// 2. Second pass: scan package archives
	// Expected format: statusline_{version}_{os}_{arch}.(tar.gz|zip)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lowerName := strings.ToLower(name)
		if !strings.HasPrefix(lowerName, "statusline_") {
			continue
		}
		if !strings.HasSuffix(lowerName, ".tar.gz") && !strings.HasSuffix(lowerName, ".tgz") && !strings.HasSuffix(lowerName, ".zip") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		ver, platform := parsePackageName(name)
		if ver == "" || platform == "" {
			continue
		}

		checksum := catalog.Checksums[name]
		dlPath := "/api/v1/upgrade/download?filename=" + name

		pkg := ReleasePackage{
			Filename:     name,
			Version:      ver,
			Platform:     platform,
			Size:         info.Size(),
			Checksum:     checksum,
			DownloadPath: dlPath,
		}

		catalog.Packages = append(catalog.Packages, pkg)
		catalog.Versions[ver] = append(catalog.Versions[ver], pkg)
	}

	if len(catalog.Packages) > 0 {
		catalog.UpgradeSupported = true

		// Find latest version
		var uniqueVersions []string
		for v := range catalog.Versions {
			uniqueVersions = append(uniqueVersions, v)
		}
		sort.Slice(uniqueVersions, func(i, j int) bool {
			return updater.CompareSemVer(uniqueVersions[i], uniqueVersions[j]) > 0
		})

		catalog.LatestVersion = uniqueVersions[0]
		for _, pkg := range catalog.Versions[catalog.LatestVersion] {
			catalog.Assets[pkg.Platform] = pkg.DownloadPath
		}
	}

	return catalog
}

// parsePackageName parses statusline_{version}_{os}_{arch}.(tar.gz|zip)
func parsePackageName(name string) (version, platform string) {
	clean := name
	if strings.HasSuffix(clean, ".tar.gz") {
		clean = strings.TrimSuffix(clean, ".tar.gz")
	} else if strings.HasSuffix(clean, ".tgz") {
		clean = strings.TrimSuffix(clean, ".tgz")
	} else if strings.HasSuffix(clean, ".zip") {
		clean = strings.TrimSuffix(clean, ".zip")
	}

	parts := strings.Split(clean, "_")
	// e.g. ["statusline", "v0.1.6", "linux", "amd64"] -> len 4
	if len(parts) >= 4 && parts[0] == "statusline" {
		version = parts[1]
		platform = parts[2] + "_" + parts[3]
		return version, platform
	}
	return "", ""
}
