package update

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	content := `abc123def456  caddy-atc_0.5.0_linux_amd64.tar.gz
789xyz000111  caddy-atc_0.5.0_darwin_arm64.tar.gz
`
	checksums := parseChecksums(content)
	if len(checksums) != 2 {
		t.Fatalf("expected 2 checksums, got %d", len(checksums))
	}
	if checksums["caddy-atc_0.5.0_linux_amd64.tar.gz"] != "abc123def456" {
		t.Errorf("wrong checksum for linux_amd64: %s", checksums["caddy-atc_0.5.0_linux_amd64.tar.gz"])
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	hash := sha256.Sum256(data)
	expected := fmt.Sprintf("%x", hash)

	if err := verifyChecksum(data, expected); err != nil {
		t.Errorf("verifyChecksum() should pass: %v", err)
	}

	if err := verifyChecksum(data, "bad"); err == nil {
		t.Error("verifyChecksum() should fail for wrong hash")
	}
}

func TestArchiveName(t *testing.T) {
	name := archiveName("0.5.0", "linux", "amd64")
	want := "caddy-atc_0.5.0_linux_amd64.tar.gz"
	if name != want {
		t.Errorf("archiveName() = %q, want %q", name, want)
	}
}

func TestValidateArchivePlatform(t *testing.T) {
	if err := validateArchivePlatform("caddy-atc_0.5.0_linux_amd64.tar.gz", "linux", "amd64"); err != nil {
		t.Errorf("validateArchivePlatform() should pass: %v", err)
	}
	if err := validateArchivePlatform("caddy-atc_0.5.0_darwin_arm64.tar.gz", "linux", "amd64"); err == nil {
		t.Error("validateArchivePlatform() should fail for wrong platform")
	}
}
