package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SelfUpdate downloads and installs the specified version, replacing the running binary.
func SelfUpdate(version string) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Strip leading v for archive name
	ver := strings.TrimPrefix(version, "v")

	archive := archiveName(ver, goos, goarch)
	if err := validateArchivePlatform(archive, goos, goarch); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, version)

	// Download checksums
	fmt.Println("Downloading checksums...")
	checksumsData, err := download(baseURL + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	checksums := parseChecksums(string(checksumsData))
	expectedHash, ok := checksums[archive]
	if !ok {
		return fmt.Errorf("archive %s not found in checksums.txt", archive)
	}

	// Download archive
	fmt.Printf("Downloading %s...\n", archive)
	archiveData, err := download(baseURL + "/" + archive)
	if err != nil {
		return fmt.Errorf("downloading archive: %w", err)
	}

	// Verify checksum
	fmt.Println("Verifying checksum...")
	if err := verifyChecksum(archiveData, expectedHash); err != nil {
		return err
	}

	// Extract binary from tar.gz
	fmt.Println("Extracting...")
	binaryData, err := extractBinary(archiveData)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// Replace running binary
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	fmt.Printf("Replacing %s...\n", exe)
	if err := replaceBinary(exe, binaryData); err != nil {
		return err
	}

	fmt.Printf("Updated to %s\n", version)
	return nil
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

func parseChecksums(content string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			m[fields[1]] = fields[0]
		}
	}
	return m
}

func verifyChecksum(data []byte, expected string) error {
	hash := sha256.Sum256(data)
	actual := fmt.Sprintf("%x", hash)
	if actual != expected {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s", expected, actual)
	}
	return nil
}

func archiveName(version, goos, goarch string) string {
	return fmt.Sprintf("caddy-atc_%s_%s_%s.tar.gz", version, goos, goarch)
}

func validateArchivePlatform(name, goos, goarch string) error {
	expected := fmt.Sprintf("_%s_%s", goos, goarch)
	if !strings.Contains(name, expected) {
		return fmt.Errorf("archive %s does not match platform %s/%s", name, goos, goarch)
	}
	return nil
}

func extractBinary(archiveData []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == "caddy-atc" || filepath.Base(hdr.Name) == "caddy-atc" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("caddy-atc binary not found in archive")
}

func replaceBinary(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "caddy-atc-update-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Cross-device fallback: copy and rename
		if copyErr := copyFile(tmpPath, path); copyErr != nil {
			os.Remove(tmpPath)
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied replacing %s (try: sudo caddy-atc update)", path)
			}
			return fmt.Errorf("replacing binary: %w", err)
		}
		os.Remove(tmpPath)
		return nil
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
