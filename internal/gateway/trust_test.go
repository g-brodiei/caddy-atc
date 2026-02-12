package gateway

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func makeTar(t *testing.T, entries []struct {
	name    string
	typeflag byte
	content []byte
}) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Size:     int64(len(e.content)),
			Mode:     0644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing tar header: %v", err)
		}
		if len(e.content) > 0 {
			if _, err := tw.Write(e.content); err != nil {
				t.Fatalf("writing tar content: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	return &buf
}

func TestExtractFromTar_ValidCert(t *testing.T) {
	certData := []byte("-----BEGIN CERTIFICATE-----\nMIIBfake...\n-----END CERTIFICATE-----\n")
	r := makeTar(t, []struct {
		name     string
		typeflag byte
		content  []byte
	}{
		{"root.crt", tar.TypeReg, certData},
	})

	got, err := extractFromTar(r, maxCertSize)
	if err != nil {
		t.Fatalf("extractFromTar() error = %v", err)
	}
	if !bytes.Equal(got, certData) {
		t.Errorf("got %q, want %q", string(got), string(certData))
	}
}

func TestExtractFromTar_EmptyTar(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.Close()

	_, err := extractFromTar(&buf, maxCertSize)
	if err == nil {
		t.Error("expected error for empty tar archive")
	}
}

func TestExtractFromTar_Oversized(t *testing.T) {
	// Create content that exceeds the limit
	maxBytes := int64(100) // small limit for test
	bigContent := bytes.Repeat([]byte("x"), int(maxBytes)+10)

	r := makeTar(t, []struct {
		name     string
		typeflag byte
		content  []byte
	}{
		{"big.crt", tar.TypeReg, bigContent},
	})

	_, err := extractFromTar(r, maxBytes)
	if err == nil {
		t.Error("expected error for oversized file")
	}
}

func TestExtractFromTar_DirectoryEntrySkipped(t *testing.T) {
	certData := []byte("real-cert-data")
	r := makeTar(t, []struct {
		name     string
		typeflag byte
		content  []byte
	}{
		{"data/", tar.TypeDir, nil},
		{"data/root.crt", tar.TypeReg, certData},
	})

	got, err := extractFromTar(r, maxCertSize)
	if err != nil {
		t.Fatalf("extractFromTar() error = %v", err)
	}
	if !bytes.Equal(got, certData) {
		t.Errorf("got %q, want %q (directory should be skipped)", string(got), string(certData))
	}
}

func TestExtractFromTar_OnlyDirectories(t *testing.T) {
	r := makeTar(t, []struct {
		name     string
		typeflag byte
		content  []byte
	}{
		{"dir1/", tar.TypeDir, nil},
		{"dir2/", tar.TypeDir, nil},
	})

	_, err := extractFromTar(r, maxCertSize)
	if err == nil {
		t.Error("expected error when tar contains only directories")
	}
}

func TestExtractFromTar_ExactlyMaxSize(t *testing.T) {
	maxBytes := int64(50)
	content := bytes.Repeat([]byte("x"), int(maxBytes))

	r := makeTar(t, []struct {
		name     string
		typeflag byte
		content  []byte
	}{
		{"exact.crt", tar.TypeReg, content},
	})

	got, err := extractFromTar(r, maxBytes)
	if err != nil {
		t.Fatalf("extractFromTar() error = %v (file exactly at limit should succeed)", err)
	}
	if int64(len(got)) != maxBytes {
		t.Errorf("got %d bytes, want %d", len(got), maxBytes)
	}
}
