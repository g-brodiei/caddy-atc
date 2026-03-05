# `caddy-atc start` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `caddy-atc start` command that transparently strips host port bindings from compose files and wraps the user's command, eliminating cross-project port conflicts.

**Architecture:** New `internal/start` package handles YAML port stripping (via `yaml.v3` Node API) and command execution (via `syscall.Exec`). The existing `adopt` and `gateway` packages are called for auto-adopt and auto-start. Two new cobra commands (`start`, `stop`) are added to `cmd/caddy-atc/main.go`.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (Node API), `syscall.Exec`, `github.com/spf13/cobra`

---

### Task 1: Port stripping — core YAML manipulation

**Files:**
- Create: `internal/start/strip.go`
- Create: `internal/start/strip_test.go`

**Step 1: Write failing tests for StripPorts**

```go
// internal/start/strip_test.go
package start

import (
	"strings"
	"testing"
)

func TestStripPorts_Basic(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    volumes:
      - ./html:/usr/share/nginx/html
  db:
    image: postgres
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: mydb
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output, got:\n%s", output)
	}
	if !strings.Contains(output, "image: nginx") {
		t.Error("expected 'image: nginx' preserved")
	}
	if !strings.Contains(output, "volumes:") {
		t.Error("expected 'volumes:' preserved")
	}
	if !strings.Contains(output, "POSTGRES_DB: mydb") {
		t.Error("expected environment preserved")
	}
}

func TestStripPorts_PreservesVariableReferences(t *testing.T) {
	input := `services:
  backend:
    build: ./backend
    ports:
      - "8000:8000"
    environment:
      - FINLAB_API_TOKEN=${FINLAB_API_TOKEN}
      - HOST_UID=${HOST_UID}
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output, got:\n%s", output)
	}
	if !strings.Contains(output, "${FINLAB_API_TOKEN}") {
		t.Error("expected ${FINLAB_API_TOKEN} preserved")
	}
	if !strings.Contains(output, "${HOST_UID}") {
		t.Error("expected ${HOST_UID} preserved")
	}
}

func TestStripPorts_PreservesExpose(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    expose:
      - "80"
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected no ports: in output")
	}
	if !strings.Contains(output, "expose:") {
		t.Error("expected expose: preserved")
	}
}

func TestStripPorts_KeepPorts(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
  db:
    image: postgres
    ports:
      - "5432:5432"
  redis:
    image: redis
    ports:
      - "6379:6379"
`
	got, err := StripPorts([]byte(input), []string{"db", "redis"})
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)

	// web ports should be stripped
	// db and redis ports should be kept
	// We need to check service-by-service, so parse minimally
	if !strings.Contains(output, "5432:5432") {
		t.Error("expected db ports preserved with --keep-ports")
	}
	if !strings.Contains(output, "6379:6379") {
		t.Error("expected redis ports preserved with --keep-ports")
	}
	// web's "80:80" should be gone
	if strings.Contains(output, "80:80") {
		t.Error("expected web ports stripped")
	}
}

func TestStripPorts_NoServices(t *testing.T) {
	input := `volumes:
  data:
    driver: local
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "volumes:") {
		t.Error("expected volumes preserved")
	}
}

func TestStripPorts_LongFormPorts(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - target: 80
        published: 8080
        protocol: tcp
    volumes:
      - ./html:/usr/share/nginx/html
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected long-form ports stripped, got:\n%s", output)
	}
	if !strings.Contains(output, "volumes:") {
		t.Error("expected volumes preserved")
	}
}

func TestStripPorts_PreservesProfiles(t *testing.T) {
	input := `services:
  caddy-dev:
    image: caddy:2-alpine
    profiles: [dev]
    ports:
      - "3000:3000"
  backend:
    build: ./backend
    ports:
      - "8000:8000"
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if strings.Contains(output, "ports:") {
		t.Errorf("expected ports stripped")
	}
	if !strings.Contains(output, "profiles:") {
		t.Error("expected profiles preserved")
	}
}

func TestStripPorts_ServiceWithNoPorts(t *testing.T) {
	input := `services:
  worker:
    build: ./backend
    command: python -m worker
    environment:
      - REDIS_URL=redis://redis:6379/0
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	// Should pass through unchanged (minus possible whitespace)
	output := string(got)
	if !strings.Contains(output, "command: python -m worker") {
		t.Error("expected service without ports to pass through unchanged")
	}
}

func TestStripPorts_PreservesNetworksAndVolumes(t *testing.T) {
	input := `services:
  web:
    image: nginx
    ports:
      - "80:80"
    networks:
      - app-network

networks:
  app-network:
    driver: bridge

volumes:
  caddy_data:
  caddy_config:
`
	got, err := StripPorts([]byte(input), nil)
	if err != nil {
		t.Fatalf("StripPorts() error = %v", err)
	}
	output := string(got)
	if !strings.Contains(output, "networks:") {
		t.Error("expected networks preserved")
	}
	if !strings.Contains(output, "app-network") {
		t.Error("expected app-network preserved")
	}
	if !strings.Contains(output, "caddy_data") {
		t.Error("expected volumes preserved")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/start/ -v -count=1`
Expected: FAIL — package doesn't exist yet.

**Step 3: Implement StripPorts**

```go
// internal/start/strip.go
package start

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// StripPorts parses a docker-compose YAML document and removes all `ports:`
// entries from services. If keepPorts is non-empty, services whose names match
// entries in keepPorts retain their ports. All other YAML content (variables,
// anchors, comments, structure) is preserved via the yaml.v3 Node API.
func StripPorts(data []byte, keepPorts []string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// Document node wraps the actual content
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return data, nil // empty or unexpected structure, return as-is
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return data, nil
	}

	keepSet := make(map[string]bool, len(keepPorts))
	for _, s := range keepPorts {
		keepSet[s] = true
	}

	// Find "services" key in the root mapping
	for i := 0; i < len(root.Content)-1; i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]

		if keyNode.Value != "services" || valNode.Kind != yaml.MappingNode {
			continue
		}

		// Iterate over each service
		for j := 0; j < len(valNode.Content)-1; j += 2 {
			svcName := valNode.Content[j].Value
			svcNode := valNode.Content[j+1]

			if keepSet[svcName] {
				continue
			}

			if svcNode.Kind != yaml.MappingNode {
				continue
			}

			stripPortsFromService(svcNode)
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("marshaling YAML: %w", err)
	}
	return out, nil
}

// stripPortsFromService removes the "ports" key-value pair from a service
// mapping node.
func stripPortsFromService(svc *yaml.Node) {
	filtered := make([]*yaml.Node, 0, len(svc.Content))
	for i := 0; i < len(svc.Content)-1; i += 2 {
		if svc.Content[i].Value == "ports" {
			continue // skip this key-value pair
		}
		filtered = append(filtered, svc.Content[i], svc.Content[i+1])
	}
	svc.Content = filtered
}
```

**Step 4: Run tests to verify they pass**

Run: `/usr/local/go/bin/go test ./internal/start/ -v -count=1`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/start/strip.go internal/start/strip_test.go
git commit -m "feat(start): add YAML port stripping with Node API

Preserves variable references, expose directives, profiles,
networks, volumes. Supports --keep-ports for selective retention.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Compose file detection and stripped file generation

**Files:**
- Create: `internal/start/compose.go`
- Create: `internal/start/compose_test.go`

**Step 1: Write failing tests for compose file detection and generation**

```go
// internal/start/compose_test.go
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

	// Verify the stripped file exists and has no ports
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

	// Verify filename convention
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

	// Override stripped file should not have ports
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
```

**Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/start/ -v -count=1 -run TestDetect`
Expected: FAIL — functions don't exist yet.

**Step 3: Implement compose file detection and generation**

```go
// internal/start/compose.go
package start

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// strippedPrefix is prepended to compose filenames for the stripped copies.
const strippedPrefix = ".caddy-atc-compose"

// DetectComposeFiles finds which compose files Docker Compose would load
// for the given project directory. Checks COMPOSE_FILE env var first, then
// falls back to standard file detection (with override auto-loading).
func DetectComposeFiles(dir string) ([]string, error) {
	// If COMPOSE_FILE is set, use those files
	if envVal := os.Getenv("COMPOSE_FILE"); envVal != "" {
		sep := ":"
		if os.PathSeparator == '\\' {
			sep = ";"
		}
		parts := strings.Split(envVal, sep)
		var files []string
		for _, p := range parts {
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("COMPOSE_FILE references missing file: %s", p)
			}
			files = append(files, p)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("COMPOSE_FILE is set but contains no valid files")
		}
		return files, nil
	}

	// Standard detection: find base file
	base := findBaseComposeFile(dir)
	if base == "" {
		return nil, fmt.Errorf("no docker-compose.yml or compose.yml found in %s", dir)
	}

	files := []string{base}

	// Check for auto-loaded override file
	override := findOverrideFile(dir, base)
	if override != "" {
		files = append(files, override)
	}

	return files, nil
}

// GenerateStrippedFiles creates port-stripped copies of the given compose files.
// Returns the paths to the stripped files in the same order.
func GenerateStrippedFiles(originals []string, keepPorts []string) ([]string, error) {
	var stripped []string

	for i, orig := range originals {
		data, err := os.ReadFile(orig)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", orig, err)
		}

		out, err := StripPorts(data, keepPorts)
		if err != nil {
			return nil, fmt.Errorf("stripping ports from %s: %w", orig, err)
		}

		dir := filepath.Dir(orig)
		name := strippedFilename(i, len(originals))
		outPath := filepath.Join(dir, name)

		if err := os.WriteFile(outPath, out, 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", outPath, err)
		}

		stripped = append(stripped, outPath)
	}

	return stripped, nil
}

// BuildComposeFileEnv builds the COMPOSE_FILE env var value from file paths.
func BuildComposeFileEnv(files []string) string {
	return strings.Join(files, ":")
}

func findBaseComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findOverrideFile(dir string, base string) string {
	baseName := filepath.Base(base)
	ext := filepath.Ext(baseName)
	nameNoExt := strings.TrimSuffix(baseName, ext)

	// docker-compose.yml -> docker-compose.override.yml
	// compose.yml -> compose.override.yml
	override := nameNoExt + ".override" + ext
	p := filepath.Join(dir, override)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func strippedFilename(index int, total int) string {
	if total == 1 {
		return strippedPrefix + ".yml"
	}
	if index == 0 {
		return strippedPrefix + ".yml"
	}
	return strippedPrefix + ".override.yml"
}
```

**Step 4: Run tests to verify they pass**

Run: `/usr/local/go/bin/go test ./internal/start/ -v -count=1`
Expected: All PASS.

**Step 5: Commit**

```bash
git add internal/start/compose.go internal/start/compose_test.go
git commit -m "feat(start): add compose file detection and stripped file generation

Detects base + override files, respects COMPOSE_FILE env var.
Generates .caddy-atc-compose.yml stripped copies.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Start command core logic

**Files:**
- Create: `internal/start/start.go`

**Step 1: Write the start logic**

Note: This code orchestrates auto-adopt, gateway start, file stripping, env setup, and exec. It depends on Tasks 1 and 2. Testing is primarily integration — the units are tested in Tasks 1 and 2.

```go
// internal/start/start.go
package start

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/g-brodiei/caddy-atc/internal/adopt"
	"github.com/g-brodiei/caddy-atc/internal/config"
	"github.com/g-brodiei/caddy-atc/internal/gateway"
)

// Options configures the start command.
type Options struct {
	Dir       string   // Project directory (resolved to absolute)
	KeepPorts []string // Service names whose ports should be kept
	Command   []string // User command to run (nil = docker compose up -d)
}

// Run executes the start workflow: auto-adopt, ensure gateway, strip ports, exec command.
func Run(ctx context.Context, opts Options) error {
	// Resolve project directory
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}

	// 1. Auto-adopt if not already adopted
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectName := filepath.Base(absDir)
	if _, ok := cfg.Projects[projectName]; !ok {
		fmt.Printf("Auto-adopting %s (%s.localhost)...\n", projectName, projectName)
		if _, err := adopt.Adopt(absDir, "", false); err != nil {
			return fmt.Errorf("auto-adopt failed: %w", err)
		}
	}

	// 2. Ensure gateway is running
	running, err := gateway.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("checking gateway: %w", err)
	}
	if !running {
		fmt.Println("Starting caddy-atc gateway...")
		if err := gateway.Up(ctx); err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
	}

	// 3. Detect compose files
	composeFiles, err := DetectComposeFiles(absDir)
	if err != nil {
		return err
	}

	// 4. Generate stripped files
	strippedFiles, err := GenerateStrippedFiles(composeFiles, opts.KeepPorts)
	if err != nil {
		return err
	}

	fmt.Printf("Generated %s (ports stripped)\n", filepath.Base(strippedFiles[0]))

	// 5. Build environment with COMPOSE_FILE pointing to stripped files
	composeFileEnv := BuildComposeFileEnv(strippedFiles)
	env := config.FilterEnv("COMPOSE_FILE")
	env = append(env, "COMPOSE_FILE="+composeFileEnv)

	// 6. Execute command
	if len(opts.Command) == 0 {
		return runDefault(ctx, absDir, env)
	}

	return execUserCommand(absDir, env, opts.Command)
}

// runDefault runs `docker compose up -d` and returns.
func runDefault(ctx context.Context, dir string, env []string) error {
	fmt.Println("Running: docker compose up -d")

	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d")
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Println("\nContainers started. The caddy-atc watcher will set up routes automatically.")
	fmt.Println("Tip: Add .caddy-atc-compose*.yml to your .gitignore")
	return nil
}

// execUserCommand replaces the current process with the user's command.
func execUserCommand(dir string, env []string, args []string) error {
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", args[0])
	}

	// Change to project directory before exec
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("changing to project directory: %w", err)
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))

	// syscall.Exec replaces the process — signals go directly to the new command
	return syscall.Exec(binary, args, env)
}

// Stop runs docker compose down using the stripped compose file.
func Stop(ctx context.Context, dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}

	strippedPath := filepath.Join(absDir, strippedPrefix+".yml")
	if _, err := os.Stat(strippedPath); os.IsNotExist(err) {
		// No stripped file — fall back to plain docker compose down
		fmt.Println("No stripped compose file found. Running: docker compose down")
		cmd := exec.CommandContext(ctx, "docker", "compose", "down")
		cmd.Dir = absDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	env := config.FilterEnv("COMPOSE_FILE")
	env = append(env, "COMPOSE_FILE="+strippedPath)

	// Also check for override
	overridePath := filepath.Join(absDir, strippedPrefix+".override.yml")
	if _, err := os.Stat(overridePath); err == nil {
		env[len(env)-1] = "COMPOSE_FILE=" + strippedPath + ":" + overridePath
	}

	fmt.Println("Running: docker compose down")
	cmd := exec.CommandContext(ctx, "docker", "compose", "down")
	cmd.Dir = absDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}

	// Clean up stripped files
	os.Remove(strippedPath)
	os.Remove(overridePath)
	fmt.Println("Stripped compose files cleaned up.")

	return nil
}
```

**Step 2: Verify it compiles**

Run: `/usr/local/go/bin/go build ./internal/start/`
Expected: exit 0 (no errors).

**Step 3: Commit**

```bash
git add internal/start/start.go
git commit -m "feat(start): add start/stop orchestration logic

Auto-adopt, auto-start gateway, strip ports, exec user command.
Uses syscall.Exec for signal forwarding on custom commands.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Wire up cobra commands in main.go

**Files:**
- Modify: `cmd/caddy-atc/main.go`

**Step 1: Add start and stop commands**

Add to `main()` after existing `rootCmd.AddCommand` calls:

```go
rootCmd.AddCommand(startCmd())
rootCmd.AddCommand(stopCmd())
```

Add these functions:

```go
func startCmd() *cobra.Command {
	var keepPorts string

	cmd := &cobra.Command{
		Use:   "start [directory] [-- command...]",
		Short: "Start project with ports stripped (avoids conflicts)",
		Long: `Strip host port bindings from the project's compose file and run a command.

The stripped compose file is set via COMPOSE_FILE env var, so any docker compose
calls inside your script transparently use it. The caddy-atc watcher handles
routing automatically.

Examples:
  caddy-atc start                          # docker compose up -d (default)
  caddy-atc start -- ./scripts/dev.sh      # custom command
  caddy-atc start --keep-ports db,redis    # keep host ports for db and redis`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			dir := "."
			var userCmd []string

			// Split args at "--" (cobra passes everything after -- in args)
			// But cobra also handles -- specially — the args after -- are in ArgsAfterDash
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx >= 0 {
				// Everything before -- is positional args (directory)
				if dashIdx > 0 {
					dir = args[0]
				}
				// Everything after -- is the user command
				userCmd = args[dashIdx:]
			} else if len(args) > 0 {
				dir = args[0]
			}

			var keepPortsList []string
			if keepPorts != "" {
				keepPortsList = strings.Split(keepPorts, ",")
			}

			return start.Run(ctx, start.Options{
				Dir:       dir,
				KeepPorts: keepPortsList,
				Command:   userCmd,
			})
		},
	}

	cmd.Flags().StringVar(&keepPorts, "keep-ports", "", "Comma-separated service names to keep host port bindings (e.g. db,redis)")

	return cmd
}

func stopCmd_() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [directory]",
		Short: "Stop project containers and clean up stripped compose files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return start.Stop(cmd.Context(), dir)
		},
	}
}
```

Add the import:

```go
"github.com/g-brodiei/caddy-atc/internal/start"
```

**Important:** The existing `stopCmd` doesn't exist — there's a `downCmd`. So the new function should just be `stopCmd`. Rename `stopCmd_` above to `stopCmd` if there's no conflict, but double-check: the existing `downCmd()` stops the gateway. The new `stopCmd()` stops a project. These are different commands.

Verify no name conflict: existing code has `upCmd`, `downCmd`, `adoptCmd`, `unadoptCmd`, `statusCmd`, `routesCmd`, `trustCmd`, `logsCmd`. No `startCmd` or `stopCmd`. Safe to use both names.

**Step 2: Verify it compiles**

Run: `/usr/local/go/bin/go build -o /dev/null ./cmd/caddy-atc/`
Expected: exit 0.

**Step 3: Verify help output**

Run: `/usr/local/go/bin/go run ./cmd/caddy-atc/ start --help`
Expected: Shows usage, --keep-ports flag, examples.

**Step 4: Commit**

```bash
git add cmd/caddy-atc/main.go
git commit -m "feat: add 'start' and 'stop' commands to CLI

caddy-atc start strips ports and wraps user commands.
caddy-atc stop tears down containers and cleans up stripped files.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Build, verify all tests pass, update docs

**Files:**
- Modify: `README.md` (add start/stop to commands table)
- Modify: `CLAUDE.md` (add start package to project structure)

**Step 1: Run full test suite**

Run: `/usr/local/go/bin/go test ./... -count=1`
Expected: All pass (existing tests + new tests).

**Step 2: Run vet**

Run: `/usr/local/go/bin/go vet ./...`
Expected: exit 0.

**Step 3: Build binary**

Run: `/usr/local/go/bin/go build -o build/caddy-atc ./cmd/caddy-atc`
Expected: exit 0, binary at `build/caddy-atc`.

**Step 4: Verify start --help**

Run: `build/caddy-atc start --help`
Expected: Shows usage with examples and --keep-ports flag.

**Step 5: Update README.md**

Add `start` and `stop` to the Commands table:

```markdown
| `caddy-atc start [dir] [-- cmd]` | Start project with ports stripped |
| `caddy-atc stop [dir]` | Stop project and clean up stripped files |
```

Add a new "Starting Projects" section after "Commands":

```markdown
## Starting Projects

Use `caddy-atc start` to run projects without port conflicts:

\```bash
# From the project directory — uses docker compose up -d
caddy-atc start

# With a custom start script
caddy-atc start -- ./scripts/dev.sh

# Keep host ports for specific services (e.g. for pgAdmin)
caddy-atc start --keep-ports db,redis -- ./scripts/dev.sh

# Stop the project
caddy-atc stop
\```

This strips all host port bindings from the compose file and sets `COMPOSE_FILE`
so any `docker compose` calls use the stripped version. Add `.caddy-atc-compose*.yml`
to your `.gitignore`.
```

**Step 6: Update CLAUDE.md**

Add `internal/start/` to the project structure section.

**Step 7: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add start/stop command documentation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: Manual integration test

**Not committed — this is a verification step.**

Test with the real investment project:

**Step 1: Verify adopt works**

Run: `build/caddy-atc adopt --dry-run ~/project/investment`
Expected: Shows detected services with hostnames.

**Step 2: Verify start generates stripped file**

Run: `cd ~/project/investment && ~/project/caddy-atc/build/caddy-atc start -- echo "it works"`
Expected:
- Auto-adopts if needed
- Prints "Generated .caddy-atc-compose.yml (ports stripped)"
- Prints "Running: echo it works"
- Prints "it works"

**Step 3: Verify stripped file content**

Run: Read `~/project/investment/.caddy-atc-compose.yml`
Expected:
- No `ports:` entries for any service
- `${FINLAB_API_TOKEN}` preserved as variable reference
- profiles, volumes, networks, etc. all preserved

**Step 4: Clean up**

Run: `rm ~/project/investment/.caddy-atc-compose.yml`
