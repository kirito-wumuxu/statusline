package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalSource provides updates from a local releases directory (e.g. ~/.local/share/statusline/releases or ./dist).
type LocalSource struct {
	Dir string
}

// NewLocalSource creates a new LocalSource.
func NewLocalSource(dir string) *LocalSource {
	return &LocalSource{Dir: dir}
}

func (s *LocalSource) Name() string {
	return "local"
}

// CheckRelease scans s.Dir for releases matching targetVersion (or finds latest SemVer).
func (s *LocalSource) CheckRelease(ctx context.Context, targetVersion string) (*ReleaseInfo, error) {
	cleanDir := s.Dir
	if strings.HasPrefix(cleanDir, "~/") || cleanDir == "~" {
		homeDir, _ := os.UserHomeDir()
		cleanDir = filepath.Join(homeDir, cleanDir[1:])
	}
	cleanDir = filepath.Clean(cleanDir)

	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read local releases directory %s: %w", cleanDir, err)
	}

	checksums := make(map[string]string)
	// 1. Read checksums if available
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
						hash := parts[0]
						fn := filepath.Base(parts[1])
						checksums[fn] = hash
					}
				}
				_ = f.Close()
			}
		}
	}

	// 2. Scan release packages: statusline_{version}_{os}_{arch}.(tar.gz|zip)
	type pkgItem struct {
		filename string
		version  string
		platform string
		path     string
	}
	var pkgs []pkgItem
	versionMap := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "statusline_") {
			continue
		}
		isArchive := strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".zip")
		if !isArchive {
			continue
		}

		// Strip extension
		base := name
		if strings.HasSuffix(base, ".tar.gz") {
			base = strings.TrimSuffix(base, ".tar.gz")
		} else if strings.HasSuffix(base, ".tgz") {
			base = strings.TrimSuffix(base, ".tgz")
		} else if strings.HasSuffix(base, ".zip") {
			base = strings.TrimSuffix(base, ".zip")
		}

		parts := strings.Split(base, "_")
		if len(parts) < 4 {
			continue
		}
		ver := parts[1]
		platform := parts[2] + "_" + parts[3]

		pkgs = append(pkgs, pkgItem{
			filename: name,
			version:  ver,
			platform: platform,
			path:     filepath.Join(cleanDir, name),
		})
		versionMap[ver] = true
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no release packages found in %s", cleanDir)
	}

	// Determine selected version
	selectedVer := ""
	if targetVersion != "" && targetVersion != "latest" {
		targetClean := CleanVersion(targetVersion)
		for v := range versionMap {
			if CleanVersion(v) == targetClean {
				selectedVer = v
				break
			}
		}
		if selectedVer == "" {
			return nil, fmt.Errorf("target version %s not found in local directory %s", targetVersion, cleanDir)
		}
	} else {
		// Find highest SemVer
		for v := range versionMap {
			if selectedVer == "" || IsNewer(v, selectedVer) {
				selectedVer = v
			}
		}
	}

	assets := make(map[string]string)
	for _, p := range pkgs {
		if p.version == selectedVer {
			assets[p.platform] = p.path
		}
	}

	if len(assets) == 0 {
		return nil, fmt.Errorf("no assets found for version %s", selectedVer)
	}

	return &ReleaseInfo{
		Version:      selectedVer,
		TagName:      selectedVer,
		ReleaseNotes: fmt.Sprintf("Local release package from %s", cleanDir),
		Assets:       assets,
		Checksums:    checksums,
		Source:       "local",
	}, nil
}

// Download copies the local file to destPath with progress reporting.
func (s *LocalSource) Download(ctx context.Context, assetPath, destPath string, progressFn func(downloaded, total int64)) error {
	fi, err := os.Stat(assetPath)
	if err != nil {
		return fmt.Errorf("local asset not found at %s: %w", assetPath, err)
	}

	total := fi.Size()
	if progressFn != nil {
		progressFn(0, total)
	}

	if filepath.Clean(assetPath) == filepath.Clean(destPath) {
		if progressFn != nil {
			progressFn(total, total)
		}
		return nil
	}

	if err := copyFile(assetPath, destPath, 0644); err != nil {
		return fmt.Errorf("failed to copy local release asset: %w", err)
	}

	if progressFn != nil {
		progressFn(total, total)
	}
	return nil
}
