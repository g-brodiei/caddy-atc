package adopt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractContainerPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple port", "80", "80"},
		{"host:container", "8080:80", "80"},
		{"ip:host:container", "127.0.0.1:8080:80", "80"},
		{"range", "8000-8100", "8000"},
		{"with protocol", "80/tcp", "80"},
		{"host:container with protocol", "8080:80/tcp", "80"},
		{"ip:host:container with protocol", "127.0.0.1:8080:80/tcp", "80"},
		{"empty", "", ""},
		{"invalid", "abc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContainerPort(tt.input)
			if got != tt.expected {
				t.Errorf("extractContainerPort(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractImageBase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple with tag", "caddy:2-alpine", "caddy"},
		{"registry prefix", "registry.io/org/app:v1", "app"},
		{"no tag", "nginx", "nginx"},
		{"docker hub path", "library/redis:7", "redis"},
		{"empty", "", ""},
		{"just tag", ":latest", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImageBase(tt.input)
			if got != tt.expected {
				t.Errorf("extractImageBase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestAnalyzeService_CaddyImage(t *testing.T) {
	svc := composeServiceDef{Image: "caddy:2-alpine"}
	cs := analyzeService("web", svc)
	if !cs.IsHTTP {
		t.Error("expected caddy image to be detected as HTTP")
	}
	if cs.Port != "80" {
		t.Errorf("Port = %q, want %q", cs.Port, "80")
	}
}

func TestAnalyzeService_PostgresImage(t *testing.T) {
	svc := composeServiceDef{
		Image: "postgres:16",
		Ports: []string{"5432:5432"},
	}
	cs := analyzeService("db", svc)
	if cs.IsHTTP {
		t.Error("postgres image should not be detected as HTTP")
	}
}

func TestAnalyzeService_BuildWithPort(t *testing.T) {
	svc := composeServiceDef{
		Build: ".",
		Ports: []string{"3000:3000"},
	}
	cs := analyzeService("app", svc)
	if !cs.IsHTTP {
		t.Error("build context with port should be detected as HTTP")
	}
	if cs.Port != "3000" {
		t.Errorf("Port = %q, want %q", cs.Port, "3000")
	}
}

func TestAnalyzeService_NoPorts(t *testing.T) {
	svc := composeServiceDef{Image: "busybox"}
	cs := analyzeService("worker", svc)
	if cs.IsHTTP {
		t.Error("service with no ports should not be detected as HTTP")
	}
}

func TestAnalyzeService_NonHTTPServiceName(t *testing.T) {
	// Even with ports, a service named "redis" (matching nonHTTPImages) should not be HTTP
	svc := composeServiceDef{
		Image: "custom-image",
		Ports: []string{"80:80"},
	}
	cs := analyzeService("redis", svc)
	if cs.IsHTTP {
		t.Error("service named 'redis' should not be detected as HTTP")
	}
}

func TestAnalyzeService_UnknownImageWithHTTPPort(t *testing.T) {
	svc := composeServiceDef{
		Image: "myapp:latest",
		Ports: []string{"8080:8080"},
	}
	cs := analyzeService("api", svc)
	if !cs.IsHTTP {
		t.Error("service with known HTTP port should be detected as HTTP")
	}
	if cs.Port != "8080" {
		t.Errorf("Port = %q, want %q", cs.Port, "8080")
	}
}

func TestAnalyzeService_ExposeDirective(t *testing.T) {
	svc := composeServiceDef{
		Image:  "myapp:latest",
		Expose: []string{"3000"},
	}
	cs := analyzeService("api", svc)
	if !cs.IsHTTP {
		t.Error("service with exposed HTTP port should be detected as HTTP")
	}
}

func TestScanComposeFile(t *testing.T) {
	tmpDir := t.TempDir()

	composeContent := `services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
  api:
    build: .
    ports:
      - "3000:3000"
  db:
    image: postgres:16
    ports:
      - "5432:5432"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	services, err := ScanComposeFile(tmpDir)
	if err != nil {
		t.Fatalf("ScanComposeFile() error = %v", err)
	}

	// Should be sorted by name: api, db, web
	if len(services) != 3 {
		t.Fatalf("expected 3 services, got %d", len(services))
	}
	if services[0].Name != "api" {
		t.Errorf("services[0].Name = %q, want %q", services[0].Name, "api")
	}
	if services[1].Name != "db" {
		t.Errorf("services[1].Name = %q, want %q", services[1].Name, "db")
	}
	if services[2].Name != "web" {
		t.Errorf("services[2].Name = %q, want %q", services[2].Name, "web")
	}

	// api: build + port → HTTP
	if !services[0].IsHTTP {
		t.Error("api should be HTTP (build + port)")
	}
	// db: postgres → not HTTP
	if services[1].IsHTTP {
		t.Error("db (postgres) should not be HTTP")
	}
	// web: nginx → HTTP
	if !services[2].IsHTTP {
		t.Error("web (nginx) should be HTTP")
	}
}

func TestScanComposeFile_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ScanComposeFile(tmpDir)
	if err == nil {
		t.Error("expected error when no compose file exists")
	}
}

func TestScanComposeFile_ComposeYml(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `services:
  web:
    image: nginx
    ports:
      - "80:80"
`
	// Use compose.yml variant
	if err := os.WriteFile(filepath.Join(tmpDir, "compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	services, err := ScanComposeFile(tmpDir)
	if err != nil {
		t.Fatalf("ScanComposeFile() error = %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Name != "web" {
		t.Errorf("Name = %q, want %q", services[0].Name, "web")
	}
}
