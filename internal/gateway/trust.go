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

	fmt.Println("\nFor Windows browsers, also install the cert on the Windows side:")
	fmt.Printf("  1. Copy the cert: cp %s /mnt/c/Users/<your-user>/caddy-atc-root-ca.crt\n", certPath)
	fmt.Println("  2. Open the .crt file in Windows and click 'Install Certificate'")
	fmt.Println("  3. Choose 'Local Machine' -> 'Trusted Root Certification Authorities'")
	fmt.Println("\n  Or use PowerShell (admin):")
	fmt.Println("  Import-Certificate -FilePath C:\\Users\\<your-user>\\caddy-atc-root-ca.crt -CertStoreLocation Cert:\\LocalMachine\\Root")

	return nil
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
