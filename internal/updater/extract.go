package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// VerifySHA256 computes the SHA256 of filePath and compares it with expectedHex (case-insensitive).
func VerifySHA256(filePath, expectedHex string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for sha256 verification: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to read file for sha256 computation: %w", err)
	}

	actualHex := hex.EncodeToString(hasher.Sum(nil))
	expected := strings.TrimSpace(strings.ToLower(expectedHex))
	actual := strings.ToLower(actualHex)

	if actual != expected {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// ExtractBinaryFromArchive extracts targetBinaryName from archivePath (.tar.gz, .tgz, or .zip)
// and writes it to destPath with 0755 permissions.
func ExtractBinaryFromArchive(archivePath, targetBinaryName, destPath string) error {
	lowerPath := strings.ToLower(archivePath)
	if strings.HasSuffix(lowerPath, ".zip") {
		return extractFromZip(archivePath, targetBinaryName, destPath)
	}
	if strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz") {
		return extractFromTarGz(archivePath, targetBinaryName, destPath)
	}
	return fmt.Errorf("unsupported archive format: %s (expected .tar.gz or .zip)", archivePath)
}

func extractFromTarGz(archivePath, targetBinaryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open tar.gz archive: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar archive: %w", err)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		// Match basename (e.g. "statusline_v0.1.6/statusline" matches "statusline")
		if filepath.Base(header.Name) == targetBinaryName {
			return writeFileStream(tarReader, destPath, 0755)
		}
	}

	return fmt.Errorf("binary %q not found inside archive %s", targetBinaryName, filepath.Base(archivePath))
}

func extractFromZip(archivePath, targetBinaryName, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(file.Name) == targetBinaryName {
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open file in zip: %w", err)
			}
			defer rc.Close()
			return writeFileStream(rc, destPath, 0755)
		}
	}

	return fmt.Errorf("binary %q not found inside zip %s", targetBinaryName, filepath.Base(archivePath))
}

func writeFileStream(src io.Reader, destPath string, mode os.FileMode) error {
	_ = os.MkdirAll(filepath.Dir(destPath), 0755)
	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return fmt.Errorf("failed to write destination file %s: %w", destPath, err)
	}
	return nil
}
