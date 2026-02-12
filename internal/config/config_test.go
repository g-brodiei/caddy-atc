package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "foo.localhost", false},
		{"valid with hyphen", "my-app.localhost", false},
		{"valid with underscore", "my_app.test", false},
		{"valid numeric start", "1foo.localhost", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 254), true},
		{"max length", strings.Repeat("a", 253), false},
		{"curly brace open", "foo{bar", true},
		{"curly brace close", "foo}bar", true},
		{"newline", "foo\nbar", true},
		{"space", "foo bar", true},
		{"leading hyphen", "-foo.localhost", true},
		{"leading dot", ".foo.localhost", true},
		{"semicolon", "foo;bar", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my-container_1", false},
		{"valid numeric", "1container", false},
		{"empty", "", true},
		{"injection chars", "}\nmalicious{", true},
		{"space", "my container", true},
		{"curly brace", "container{1}", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContainerName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"port 80", "80", false},
		{"port 3000", "3000", false},
		{"port 65535", "65535", false},
		{"empty", "", true},
		{"letters", "abc", true},
		{"injection", "80;rm", true},
		{"with space", "80 ", true},
		{"negative", "-1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestResolveHostname(t *testing.T) {
	p := &ProjectConfig{
		Hostname: "myapp.localhost",
		Services: map[string]string{
			"web": "myapp.localhost",
			"api": "api.myapp.localhost",
		},
	}

	tests := []struct {
		name        string
		serviceName string
		want        string
	}{
		{"explicit mapping - web", "web", "myapp.localhost"},
		{"explicit mapping - api", "api", "api.myapp.localhost"},
		{"auto-generated", "worker", "worker.myapp.localhost"},
		{"auto-generated db", "db", "db.myapp.localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.ResolveHostname(tt.serviceName)
			if got != tt.want {
				t.Errorf("ResolveHostname(%q) = %q, want %q", tt.serviceName, got, tt.want)
			}
		})
	}
}

func TestFindProjectByComposeProject(t *testing.T) {
	cfg := &Config{
		Projects: map[string]*ProjectConfig{
			"myapp": {
				Dir:            "/home/user/myapp",
				ComposeProject: "myapp",
				Hostname:       "myapp.localhost",
			},
			"other": {
				Dir:            "/home/user/other",
				ComposeProject: "other-project",
				Hostname:       "other.localhost",
			},
		},
	}

	t.Run("found", func(t *testing.T) {
		name, proj := cfg.FindProjectByComposeProject("other-project")
		if name != "other" {
			t.Errorf("got name %q, want %q", name, "other")
		}
		if proj == nil {
			t.Fatal("expected project, got nil")
		}
		if proj.Hostname != "other.localhost" {
			t.Errorf("got hostname %q, want %q", proj.Hostname, "other.localhost")
		}
	})

	t.Run("not found", func(t *testing.T) {
		name, proj := cfg.FindProjectByComposeProject("nonexistent")
		if name != "" {
			t.Errorf("got name %q, want empty", name)
		}
		if proj != nil {
			t.Errorf("expected nil project, got %+v", proj)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		empty := &Config{Projects: make(map[string]*ProjectConfig)}
		name, proj := empty.FindProjectByComposeProject("anything")
		if name != "" || proj != nil {
			t.Errorf("expected empty result, got name=%q proj=%v", name, proj)
		}
	})
}

func TestFilterEnv(t *testing.T) {
	// Save and restore original env
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range origEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	os.Clearenv()
	os.Setenv("FOO", "bar")
	os.Setenv("BAZ", "qux")
	os.Setenv("PATH", "/usr/bin")

	t.Run("removes matching key", func(t *testing.T) {
		result := FilterEnv("FOO")
		for _, e := range result {
			if strings.HasPrefix(e, "FOO=") {
				t.Errorf("FilterEnv should have removed FOO, but found %q", e)
			}
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		result := FilterEnv("foo")
		for _, e := range result {
			if strings.HasPrefix(strings.ToUpper(e), "FOO=") {
				t.Errorf("FilterEnv should have removed FOO (case-insensitive), but found %q", e)
			}
		}
	})

	t.Run("preserves unrelated", func(t *testing.T) {
		result := FilterEnv("FOO")
		foundBaz := false
		foundPath := false
		for _, e := range result {
			if strings.HasPrefix(e, "BAZ=") {
				foundBaz = true
			}
			if strings.HasPrefix(e, "PATH=") {
				foundPath = true
			}
		}
		if !foundBaz {
			t.Error("FilterEnv removed BAZ, which should be preserved")
		}
		if !foundPath {
			t.Error("FilterEnv removed PATH, which should be preserved")
		}
	})
}

func TestLoadSaveRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Load from non-existent file should return empty config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("expected empty projects, got %d", len(cfg.Projects))
	}

	// Add a project and save
	cfg.Projects["testproject"] = &ProjectConfig{
		Dir:            "/tmp/testproject",
		ComposeProject: "testproject",
		Hostname:       "testproject.localhost",
		Services: map[string]string{
			"web": "testproject.localhost",
			"api": "api.testproject.localhost",
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load again and verify
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() after save error = %v", err)
	}
	if len(cfg2.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(cfg2.Projects))
	}
	proj := cfg2.Projects["testproject"]
	if proj == nil {
		t.Fatal("project 'testproject' not found after round-trip")
	}
	if proj.Dir != "/tmp/testproject" {
		t.Errorf("Dir = %q, want %q", proj.Dir, "/tmp/testproject")
	}
	if proj.Hostname != "testproject.localhost" {
		t.Errorf("Hostname = %q, want %q", proj.Hostname, "testproject.localhost")
	}
	if proj.Services["web"] != "testproject.localhost" {
		t.Errorf("Services[web] = %q, want %q", proj.Services["web"], "testproject.localhost")
	}
	if proj.Services["api"] != "api.testproject.localhost" {
		t.Errorf("Services[api] = %q, want %q", proj.Services["api"], "api.testproject.localhost")
	}
}

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	content := []byte("hello, world!")

	if err := atomicWriteFile(path, content, 0644); err != nil {
		t.Fatalf("atomicWriteFile() error = %v", err)
	}

	// Check content
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", string(got), string(content))
	}

	// Check permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("permissions = %o, want %o", info.Mode().Perm(), 0644)
	}
}

func TestLoadAndModifyConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Initialize empty config
	cfg := &Config{Projects: make(map[string]*ProjectConfig)}
	if err := cfg.Save(); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	// Run concurrent modifications
	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = LoadAndModify(func(cfg *Config) error {
				name := strings.Repeat("x", idx+1) // unique name per goroutine
				cfg.Projects[name] = &ProjectConfig{
					Dir:            "/tmp/" + name,
					ComposeProject: name,
					Hostname:       name + ".localhost",
				}
				return nil
			})
		}(i)
	}
	wg.Wait()

	// Check no errors
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	// Load final config and verify all projects present
	final, err := Load()
	if err != nil {
		t.Fatalf("final Load() error = %v", err)
	}
	if len(final.Projects) != n {
		t.Errorf("expected %d projects, got %d (data lost due to race)", n, len(final.Projects))
	}
}
