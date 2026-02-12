package adopt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindPrimaryService(t *testing.T) {
	tests := []struct {
		name     string
		services []ComposeService
		wantIdx  int
	}{
		{
			name: "caddy image wins",
			services: []ComposeService{
				{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
				{Name: "web", Image: "caddy:2", Port: "80", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "nginx fallback",
			services: []ComposeService{
				{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
				{Name: "proxy", Image: "nginx:latest", Port: "80", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "web name priority",
			services: []ComposeService{
				{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
				{Name: "web", Image: "node:18", Port: "8080", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "app name priority",
			services: []ComposeService{
				{Name: "worker", Image: "node:18", Port: "3000", IsHTTP: true},
				{Name: "app", Image: "node:18", Port: "8080", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "port 80 service",
			services: []ComposeService{
				{Name: "api", Image: "myimage", Port: "3000", IsHTTP: true},
				{Name: "server", Image: "myimage", Port: "80", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "default index 0",
			services: []ComposeService{
				{Name: "svc1", Image: "myimage", Port: "3000", IsHTTP: true},
				{Name: "svc2", Image: "myimage", Port: "8080", IsHTTP: true},
			},
			wantIdx: 0,
		},
		{
			name: "httpd image",
			services: []ComposeService{
				{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
				{Name: "frontend", Image: "httpd:2.4", Port: "80", IsHTTP: true},
			},
			wantIdx: 1,
		},
		{
			name: "single service",
			services: []ComposeService{
				{Name: "web", Image: "node:18", Port: "3000", IsHTTP: true},
			},
			wantIdx: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindPrimaryService(tt.services)
			if got != tt.wantIdx {
				t.Errorf("FindPrimaryService() = %d, want %d", got, tt.wantIdx)
			}
		})
	}
}

func TestAssignHostnames(t *testing.T) {
	t.Run("primary gets base", func(t *testing.T) {
		services := []ComposeService{
			{Name: "web", Image: "nginx", Port: "80", IsHTTP: true},
			{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
		}
		hostnames := assignHostnames(services, "myapp.localhost")

		// web (index 0) is primary because nginx image
		// FindPrimaryService returns 0 for nginx
		if hostnames["web"] != "myapp.localhost" {
			t.Errorf("web hostname = %q, want %q", hostnames["web"], "myapp.localhost")
		}
	})

	t.Run("others get prefixed", func(t *testing.T) {
		services := []ComposeService{
			{Name: "web", Image: "nginx", Port: "80", IsHTTP: true},
			{Name: "api", Image: "node:18", Port: "3000", IsHTTP: true},
		}
		hostnames := assignHostnames(services, "myapp.localhost")
		if hostnames["api"] != "api.myapp.localhost" {
			t.Errorf("api hostname = %q, want %q", hostnames["api"], "api.myapp.localhost")
		}
	})

	t.Run("single service gets base", func(t *testing.T) {
		services := []ComposeService{
			{Name: "app", Image: "node:18", Port: "3000", IsHTTP: true},
		}
		hostnames := assignHostnames(services, "myapp.localhost")
		if hostnames["app"] != "myapp.localhost" {
			t.Errorf("app hostname = %q, want %q", hostnames["app"], "myapp.localhost")
		}
	})
}

func TestAdopt_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a project directory with docker-compose.yml
	projectDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  web:
    image: nginx
    ports:
      - "80:80"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	result, err := Adopt(projectDir, "myproject.localhost", true)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if result.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", result.ProjectName, "myproject")
	}
	if result.Hostname != "myproject.localhost" {
		t.Errorf("Hostname = %q, want %q", result.Hostname, "myproject.localhost")
	}
	if len(result.HTTPServices) != 1 {
		t.Errorf("HTTPServices count = %d, want 1", len(result.HTTPServices))
	}

	// Dry run should NOT create a config file
	configPath := filepath.Join(tmpDir, ".caddy-atc", "projects.yml")
	if _, err := os.Stat(configPath); err == nil {
		t.Error("dry run should not create config file")
	}
}

func TestAdopt_ValidationRejectsSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	projectDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  web:
    image: nginx
    ports:
      - "80:80"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	_, err := Adopt(projectDir, "my project.localhost", false)
	if err == nil {
		t.Error("expected error for hostname with spaces")
	}
}

func TestAdopt_DefaultHostname(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	projectDir := filepath.Join(tmpDir, "coolapp")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  web:
    image: nginx
    ports:
      - "80:80"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	result, err := Adopt(projectDir, "", true)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if result.Hostname != "coolapp.localhost" {
		t.Errorf("Hostname = %q, want %q (auto-generated from dir name)", result.Hostname, "coolapp.localhost")
	}
}

func TestAdopt_NoComposeFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	projectDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	_, err := Adopt(projectDir, "empty.localhost", false)
	if err == nil {
		t.Error("expected error when no compose file exists")
	}
}

func TestAdopt_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	os.WriteFile(filePath, []byte("hello"), 0644)

	_, err := Adopt(filePath, "test.localhost", false)
	if err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestUnadopt_NotAdopted(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	projectDir := filepath.Join(tmpDir, "noproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	err := Unadopt(projectDir)
	if err == nil {
		t.Error("expected error when unadopting a project that isn't adopted")
	}
}

func TestAdopt_NoHTTPServices(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	projectDir := filepath.Join(tmpDir, "dbonly")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := `services:
  db:
    image: postgres:16
    ports:
      - "5432:5432"
`
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	_, err := Adopt(projectDir, "dbonly.localhost", false)
	if err == nil {
		t.Error("expected error when no HTTP services detected")
	}
}
