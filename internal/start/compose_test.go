package start

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectComposeFiles_Single(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)

	files, err := DetectComposeFiles(dir)
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if filepath.Base(files[0]) != "docker-compose.yml" {
		t.Errorf("expected docker-compose.yml, got %s", filepath.Base(files[0]))
	}
}

func TestDetectComposeFiles_WithOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)
	os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), []byte("services:\n  web:\n    ports:\n      - \"80:80\"\n"), 0644)

	files, err := DetectComposeFiles(dir)
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestDetectComposeFiles_ComposeYml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)

	files, err := DetectComposeFiles(dir)
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestDetectComposeFiles_NoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := DetectComposeFiles(dir)
	if err == nil {
		t.Error("expected error when no compose file exists")
	}
}

func TestDetectComposeFiles_FromEnv(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "base.yml")
	f2 := filepath.Join(dir, "extra.yml")
	os.WriteFile(f1, []byte("services:\n  web:\n    image: nginx\n"), 0644)
	os.WriteFile(f2, []byte("services:\n  db:\n    image: postgres\n"), 0644)

	t.Setenv("COMPOSE_FILE", f1+":"+f2)

	files, err := DetectComposeFiles(dir)
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files from COMPOSE_FILE, got %d", len(files))
	}
}

func TestGenerateStrippedFiles(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx
    ports:
      - "80:80"
  db:
    image: postgres
    ports:
      - "5432:5432"
`
	original := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(original, []byte(compose), 0644)

	stripped, err := GenerateStrippedFiles([]string{original}, nil)
	if err != nil {
		t.Fatalf("GenerateStrippedFiles() error = %v", err)
	}
	if len(stripped) != 1 {
		t.Fatalf("expected 1 stripped file, got %d", len(stripped))
	}

	data, err := os.ReadFile(stripped[0])
	if err != nil {
		t.Fatalf("reading stripped file: %v", err)
	}
	if strings.Contains(string(data), "ports:") {
		t.Error("stripped file still contains ports:")
	}
	if !strings.Contains(string(data), "image: nginx") {
		t.Error("stripped file missing nginx image")
	}
	if filepath.Base(stripped[0]) != ".caddy-atc-compose.yml" {
		t.Errorf("expected .caddy-atc-compose.yml, got %s", filepath.Base(stripped[0]))
	}
}

func TestGenerateStrippedFiles_Override(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)
	os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), []byte("services:\n  web:\n    ports:\n      - \"80:80\"\n"), 0644)

	originals := []string{
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, "docker-compose.override.yml"),
	}
	stripped, err := GenerateStrippedFiles(originals, nil)
	if err != nil {
		t.Fatalf("GenerateStrippedFiles() error = %v", err)
	}
	if len(stripped) != 2 {
		t.Fatalf("expected 2 stripped files, got %d", len(stripped))
	}

	data, err := os.ReadFile(stripped[1])
	if err != nil {
		t.Fatalf("reading stripped override: %v", err)
	}
	if strings.Contains(string(data), "ports:") {
		t.Error("stripped override still contains ports:")
	}
}

func TestBuildComposeFileEnv(t *testing.T) {
	files := []string{"/project/.caddy-atc-compose.yml", "/project/.caddy-atc-compose.override.yml"}
	got := BuildComposeFileEnv(files)
	want := "/project/.caddy-atc-compose.yml:/project/.caddy-atc-compose.override.yml"
	if got != want {
		t.Errorf("BuildComposeFileEnv() = %q, want %q", got, want)
	}
}
