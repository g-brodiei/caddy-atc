package gateway

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/docker/client"
	"github.com/g-brodiei/caddy-atc/internal/config"
)

const (
	caCertPath  = "/data/caddy/pki/authorities/local/root.crt"
	maxCertSize = 1 << 20 // 1 MB - more than enough for any CA certificate
)

// Trust extracts the Caddy root CA certificate and installs it in the system trust store.
func Trust(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	if !isContainerRunning(ctx, cli) {
		return fmt.Errorf("caddy gateway is not running - run 'caddy-atc up' first")
	}

	// Extract root CA cert from container
	reader, _, err := cli.CopyFromContainer(ctx, ContainerName, caCertPath)
	if err != nil {
		return fmt.Errorf("extracting CA cert: %w\nThe CA cert may not exist yet. Try visiting https://localhost first to trigger cert generation", err)
	}
	defer reader.Close()

	// CopyFromContainer returns a tar archive
	certData, err := extractFromTar(reader, maxCertSize)
	if err != nil {
		return fmt.Errorf("reading CA cert from archive: %w", err)
	}

	// Save to home dir
	homeDir := config.HomeDir()
	certLocalPath := filepath.Join(homeDir, "caddy-atc-root-ca.crt")
	if err := os.WriteFile(certLocalPath, certData, 0644); err != nil {
		return fmt.Errorf("saving CA cert: %w", err)
	}
	fmt.Println("CA certificate saved to:", certLocalPath)

	// Install in system trust store
	if err := installCert(certLocalPath); err != nil {
		return err
	}

	return nil
}

func installCert(certPath string) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("\nManually install the CA certificate:\n  %s\n", certPath)
		return nil
	}

	if isWSL() {
		return installCertWSL(certPath)
	}

	return installCertLinux(certPath)
}

func installCertLinux(certPath string) error {
	dest := "/usr/local/share/ca-certificates/caddy-atc-root-ca.crt"

	cmd := exec.Command("sudo", "cp", certPath, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying cert to system store (try running with sudo): %w", err)
	}

	cmd = exec.Command("sudo", "update-ca-certificates")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updating CA certificates: %w", err)
	}

	fmt.Println("CA certificate installed in system trust store.")
	return nil
}

func installCertWSL(certPath string) error {
	if err := installCertLinux(certPath); err != nil {
		fmt.Printf("Warning: Linux trust store install failed: %v\n", err)
	}

	// Resolve Windows user home if possible (for copy-paste ready commands)
	winUser := detectWindowsUser()
	userPlaceholder := "<your-windows-username>"
	if winUser != "" {
		userPlaceholder = winUser
	}
	winCertPath := fmt.Sprintf("C:\\Users\\%s\\caddy-atc-root-ca.crt", userPlaceholder)
	wslCertDest := fmt.Sprintf("/mnt/c/Users/%s/caddy-atc-root-ca.crt", userPlaceholder)

	fmt.Println()
	fmt.Println("Windows browsers (Chrome, Edge) use the Windows certificate store, not Linux's.")
	fmt.Println("To trust *.localhost certificates in your browser, install the CA cert on Windows:")
	fmt.Println()
	fmt.Println("Step 1 — Copy the certificate to the Windows filesystem:")
	fmt.Println()
	fmt.Printf("  cp %s %s\n", certPath, wslCertDest)
	fmt.Println()
	fmt.Println("Step 2 — Import into the Windows Trusted Root Certification Authorities store.")
	fmt.Println("         Run this from WSL (will open a Windows UAC prompt):")
	fmt.Println()
	fmt.Printf("  certutil.exe -addstore Root %s\n", winCertPath)
	fmt.Println()
	fmt.Println("After importing, restart your browser for the change to take effect.")

	return nil
}

// detectWindowsUser tries to find the Windows username for WSL instructions.
func detectWindowsUser() string {
	entries, err := os.ReadDir("/mnt/c/Users")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip well-known system directories
		switch strings.ToLower(name) {
		case "public", "default", "default user", "all users":
			continue
		}
		// First real user directory is likely the one
		return name
	}
	return ""
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

// extractFromTar reads the first file from a tar archive with a size limit.
func extractFromTar(r io.Reader, maxBytes int64) ([]byte, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("certificate not found in archive")
		}
		if err != nil {
			return nil, err
		}
		// Validate the entry looks like a certificate file
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, io.LimitReader(tr, maxBytes+1)); err != nil {
			return nil, err
		}
		if int64(buf.Len()) > maxBytes {
			return nil, fmt.Errorf("certificate file too large (>%d bytes)", maxBytes)
		}
		return buf.Bytes(), nil
	}
}
