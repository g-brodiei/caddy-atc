# Custom Compose File (`-f`) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `-f`/`--file` flag to `adopt` and `start` commands so users can specify a custom docker-compose file (e.g. `docker-compose.demo.yaml`), with the path persisted in project config.

**Architecture:** Add `ComposeFile` field to `ProjectConfig`. Thread the file path from CLI flags through `adopt.Adopt()` and `start.DetectComposeFiles()`. When `-f` is not given on `start`, fall back to the saved config value, then to auto-detection.

**Tech Stack:** Go, cobra (CLI), yaml.v3 (config)

---

## Chunk 1: Config + Adopt

### Task 1: Add `ComposeFile` field to `ProjectConfig`

**Files:**
- Modify: `internal/config/config.go:108-113`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test — ComposeFile round-trips through save/load**

Add to `internal/config/config_test.go`:

```go
func TestProjectConfig_ComposeFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := LoadAndModify(func(cfg *Config) error {
		cfg.Projects["myapp"] = &ProjectConfig{
			Dir:            "/home/user/myapp",
			ComposeProject: "myapp",
			Hostname:       "myapp.localhost",
			Services:       map[string]string{"web": "myapp.localhost"},
			ComposeFile:    "docker-compose.demo.yaml",
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LoadAndModify() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	proj := cfg.Projects["myapp"]
	if proj.ComposeFile != "docker-compose.demo.yaml" {
		t.Errorf("ComposeFile = %q, want %q", proj.ComposeFile, "docker-compose.demo.yaml")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestProjectConfig_ComposeFileRoundTrip -v -count=1`
Expected: FAIL — `ComposeFile` field does not exist

- [ ] **Step 3: Add `ComposeFile` field to `ProjectConfig`**

In `internal/config/config.go`, change `ProjectConfig`:

```go
type ProjectConfig struct {
	Dir            string            `yaml:"dir"`
	ComposeProject string            `yaml:"compose_project"`
	Hostname       string            `yaml:"hostname"`
	Services       map[string]string `yaml:"services"`
	ComposeFile    string            `yaml:"compose_file,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestProjectConfig_ComposeFileRoundTrip -v -count=1`
Expected: PASS

- [ ] **Step 5: Run all config tests**

Run: `/usr/local/go/bin/go test ./internal/config/ -v -count=1`
Expected: All PASS (existing tests unaffected by `omitempty`)

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add ComposeFile field to ProjectConfig"
```

---

### Task 2: Accept compose file in `adopt.ScanComposeFile` and `adopt.Adopt`

**Files:**
- Modify: `internal/adopt/detect.go:79-83` (ScanComposeFile signature)
- Modify: `internal/adopt/adopt.go:22,49` (Adopt signature, pass through)
- Test: `internal/adopt/detect_test.go`
- Test: `internal/adopt/adopt_test.go`

- [ ] **Step 1: Write failing test — ScanComposeFile with explicit file**

Add to `internal/adopt/detect_test.go`:

```go
func TestScanComposeFile_ExplicitFile(t *testing.T) {
	tmpDir := t.TempDir()

	composeContent := `services:
  web:
    image: nginx
    ports:
      - "80:80"
`
	// Write to a non-standard filename
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.demo.yaml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing compose file: %v", err)
	}

	services, err := ScanComposeFile(tmpDir, "docker-compose.demo.yaml")
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/usr/local/go/bin/go test ./internal/adopt/ -run TestScanComposeFile_ExplicitFile -v -count=1`
Expected: FAIL — too many arguments to ScanComposeFile

- [ ] **Step 3: Update `ScanComposeFile` to accept a compose file parameter**

In `internal/adopt/detect.go`, change the signature and resolution logic:

```go
// ScanComposeFile reads a docker-compose file and detects HTTP services.
// If composeFile is non-empty, it is resolved relative to dir instead of auto-detecting.
func ScanComposeFile(dir string, composeFile string) ([]ComposeService, error) {
	var composePath string
	if composeFile != "" {
		if filepath.IsAbs(composeFile) {
			composePath = composeFile
		} else {
			composePath = filepath.Join(dir, composeFile)
		}
		if _, err := os.Stat(composePath); err != nil {
			return nil, fmt.Errorf("compose file not found: %s", composePath)
		}
	} else {
		composePath = findComposeFile(dir)
		if composePath == "" {
			return nil, fmt.Errorf("no docker-compose.yml found in %s", dir)
		}
	}

	// ... rest unchanged from line 85 onward
```

- [ ] **Step 4: Fix existing callers — update `adopt.go` to pass empty string**

In `internal/adopt/adopt.go`, change `Adopt` signature and call site:

```go
// Adopt scans a project directory and registers it in the config.
func Adopt(dir string, hostname string, composeFile string, dryRun bool) (*Result, error) {
```

And update the `ScanComposeFile` call at line 49:

```go
	services, err := ScanComposeFile(absDir, composeFile)
```

Save `composeFile` in config at the `LoadAndModify` block (around line 94-101):

```go
	err = config.LoadAndModify(func(cfg *config.Config) error {
		cfg.Projects[projectName] = &config.ProjectConfig{
			Dir:            absDir,
			ComposeProject: composeProject,
			Hostname:       hostname,
			Services:       svcHostnames,
			ComposeFile:    composeFile,
		}
		return nil
	})
```

- [ ] **Step 5: Fix all existing callers of `Adopt` to pass `""`**

In `internal/start/start.go` line 40:
```go
		if _, err := adopt.Adopt(absDir, "", "", false); err != nil {
```

In `cmd/caddy-atc/main.go` line 122:
```go
			result, err := adopt.Adopt(dir, hostname, "", dryRun)
```

In `internal/adopt/adopt_test.go`, update all 6 `Adopt()` calls to add `""` as the third argument:
- Line 146: `Adopt(projectDir, "myproject.localhost", "", true)`
- Line 186: `Adopt(projectDir, "my project.localhost", "", false)`
- Line 211: `Adopt(projectDir, "", "", true)`
- Line 229: `Adopt(projectDir, "empty.localhost", "", false)`
- Line 240: `Adopt(filePath, "test.localhost", "", false)`
- Line 281: `Adopt(projectDir, "dbonly.localhost", "", false)`

- [ ] **Step 6: Fix existing `ScanComposeFile` test callers to pass `""`**

In `internal/adopt/detect_test.go`, update `TestScanComposeFile` (line 160) and `TestScanComposeFile_NoFile` (line 194) and `TestScanComposeFile_ComposeYml` (line 299):

```go
	services, err := ScanComposeFile(tmpDir, "")
```
```go
	_, err := ScanComposeFile(tmpDir, "")
```
```go
	services, err := ScanComposeFile(tmpDir, "")
```

- [ ] **Step 7: Run all adopt tests**

Run: `/usr/local/go/bin/go test ./internal/adopt/ -v -count=1`
Expected: All PASS

- [ ] **Step 8: Run full test suite**

Run: `/usr/local/go/bin/go test ./... -count=1`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/adopt/detect.go internal/adopt/adopt.go internal/adopt/detect_test.go internal/adopt/adopt_test.go internal/start/start.go cmd/caddy-atc/main.go
git commit -m "feat: accept compose file parameter in adopt and ScanComposeFile"
```

---

### Task 3: Add `-f` flag to `adopt` CLI command

**Files:**
- Modify: `cmd/caddy-atc/main.go:105-172` (adoptCmd function)

- [ ] **Step 1: Add `-f` flag to adoptCmd and pass to Adopt**

In `cmd/caddy-atc/main.go` `adoptCmd()`, add the flag variable and wire it:

```go
func adoptCmd() *cobra.Command {
	var hostname string
	var dryRun bool
	var composeFile string

	cmd := &cobra.Command{
		// ...
		RunE: func(cmd *cobra.Command, args []string) error {
			// ... existing dir logic ...

			fmt.Println("Scanning docker-compose.yml...")
			// If a custom file was specified, mention it
			if composeFile != "" {
				fmt.Printf("Using compose file: %s\n", composeFile)
			}
			fmt.Println()

			result, err := adopt.Adopt(dir, hostname, composeFile, dryRun)
			// ... rest unchanged ...
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "", "Override base hostname (default: <dirname>.localhost)")
	cmd.Flags().StringVarP(&composeFile, "file", "f", "", "Path to docker-compose file (default: auto-detect)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without saving")

	return cmd
}
```

- [ ] **Step 2: Build and verify**

Run: `/usr/local/go/bin/go build -o build/caddy-atc ./cmd/caddy-atc`
Expected: Compiles successfully

- [ ] **Step 3: Verify help output includes -f flag**

Run: `./build/caddy-atc adopt --help`
Expected: Shows `-f, --file string   Path to docker-compose file (default: auto-detect)`

- [ ] **Step 4: Commit**

```bash
git add cmd/caddy-atc/main.go
git commit -m "feat: add -f flag to adopt command"
```

---

## Chunk 2: Start command + integration

### Task 4: Accept compose file in `start.DetectComposeFiles`

**Files:**
- Modify: `internal/start/compose.go:46-79` (DetectComposeFiles signature)
- Test: `internal/start/compose_test.go`

- [ ] **Step 1: Write failing test — DetectComposeFiles with explicit file**

Add to `internal/start/compose_test.go`:

```go
func TestDetectComposeFiles_ExplicitFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.demo.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)

	files, err := DetectComposeFiles(dir, "docker-compose.demo.yaml")
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if filepath.Base(files[0]) != "docker-compose.demo.yaml" {
		t.Errorf("expected docker-compose.demo.yaml, got %s", filepath.Base(files[0]))
	}
}

func TestDetectComposeFiles_ExplicitFileWithOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.demo.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0644)
	os.WriteFile(filepath.Join(dir, "docker-compose.demo.override.yaml"), []byte("services:\n  web:\n    ports:\n      - \"80:80\"\n"), 0644)

	files, err := DetectComposeFiles(dir, "docker-compose.demo.yaml")
	if err != nil {
		t.Fatalf("DetectComposeFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestDetectComposeFiles_ExplicitFileMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := DetectComposeFiles(dir, "nonexistent.yml")
	if err == nil {
		t.Error("expected error when explicit file doesn't exist")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/start/ -run TestDetectComposeFiles_Explicit -v -count=1`
Expected: FAIL — too many arguments

- [ ] **Step 3: Update `DetectComposeFiles` to accept compose file parameter**

In `internal/start/compose.go`:

```go
// DetectComposeFiles finds which compose files Docker Compose would load
// for the given project directory. If composeFile is non-empty, it is used
// directly (resolved relative to dir) instead of auto-detecting.
// Checks COMPOSE_FILE env var first (when composeFile is empty), then
// falls back to standard file detection (with override auto-loading).
func DetectComposeFiles(dir string, composeFile string) ([]string, error) {
	if composeFile != "" {
		var base string
		if filepath.IsAbs(composeFile) {
			base = composeFile
		} else {
			base = filepath.Join(dir, composeFile)
		}
		if _, err := os.Stat(base); err != nil {
			return nil, fmt.Errorf("compose file not found: %s", base)
		}
		files := []string{base}
		override := findOverrideFile(filepath.Dir(base), base)
		if override != "" {
			files = append(files, override)
		}
		return files, nil
	}

	if envVal := os.Getenv("COMPOSE_FILE"); envVal != "" {
		// ... existing COMPOSE_FILE env var logic unchanged ...
```

- [ ] **Step 4: Fix existing callers of `DetectComposeFiles` to pass `""`**

In `internal/start/start.go` line 58:
```go
	composeFiles, err := DetectComposeFiles(absDir, "")
```

In `internal/start/compose_test.go`, update all existing test calls to pass `""` as second arg:
- `TestDetectComposeFiles_Single`: `DetectComposeFiles(dir, "")`
- `TestDetectComposeFiles_WithOverride`: `DetectComposeFiles(dir, "")`
- `TestDetectComposeFiles_ComposeYml`: `DetectComposeFiles(dir, "")`
- `TestDetectComposeFiles_NoFile`: `DetectComposeFiles(dir, "")`
- `TestDetectComposeFiles_FromEnv`: `DetectComposeFiles(dir, "")`

- [ ] **Step 5: Run all start tests**

Run: `/usr/local/go/bin/go test ./internal/start/ -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/start/compose.go internal/start/compose_test.go internal/start/start.go
git commit -m "feat: accept compose file parameter in DetectComposeFiles"
```

---

### Task 5: Wire `-f` flag and config fallback into `start` command

**Files:**
- Modify: `internal/start/start.go:18-22,25-62` (Options struct, Run function)
- Modify: `cmd/caddy-atc/main.go:306-354` (startCmd function)

- [ ] **Step 1: Add `ComposeFile` to `Options` and use it in `Run`**

In `internal/start/start.go`, update `Options`:

```go
type Options struct {
	Dir         string   // Project directory (resolved to absolute)
	KeepPorts   []string // Service names whose ports should be kept
	Command     []string // User command to run (nil = docker compose up -d)
	ComposeFile string   // Explicit compose file path (empty = auto-detect or use saved config)
}
```

In `Run`, after the auto-adopt block and before `DetectComposeFiles`, resolve the compose file from saved config if not provided:

```go
	// 2. Ensure gateway is running
	// ... existing gateway code ...

	// 3. Resolve compose file: flag > saved config > auto-detect
	composeFile := opts.ComposeFile
	if composeFile == "" {
		// Re-load config to get potentially just-saved ComposeFile
		cfg, err = config.Load()
		if err == nil {
			if proj, ok := cfg.Projects[projectName]; ok {
				composeFile = proj.ComposeFile
			}
		}
	}

	// 4. Detect compose files
	composeFiles, err := DetectComposeFiles(absDir, composeFile)
```

Also update the auto-adopt call to pass the compose file:

```go
	if _, ok := cfg.Projects[projectName]; !ok {
		fmt.Printf("Auto-adopting %s (%s.localhost)...\n", projectName, projectName)
		if _, err := adopt.Adopt(absDir, "", opts.ComposeFile, false); err != nil {
			return fmt.Errorf("auto-adopt failed: %w", err)
		}
	}
```

Renumber the remaining comments in `Run`:
- `// 3. Resolve compose file` (new block above)
- `// 4. Detect compose files` (was 3)
- `// 5. Generate stripped files` (was 4)
- `// 6. Build environment` (was 5)
- `// 7. Execute command` (was 6)

- [ ] **Step 2: Add `-f` flag to `startCmd` in main.go**

In `cmd/caddy-atc/main.go` `startCmd()`:

```go
func startCmd() *cobra.Command {
	var keepPorts string
	var composeFile string

	cmd := &cobra.Command{
		// ... existing Use, Short, Long ...
		RunE: func(cmd *cobra.Command, args []string) error {
			// ... existing dir/userCmd parsing ...

			var keepPortsList []string
			if keepPorts != "" {
				keepPortsList = strings.Split(keepPorts, ",")
			}

			return start.Run(ctx, start.Options{
				Dir:         dir,
				KeepPorts:   keepPortsList,
				Command:     userCmd,
				ComposeFile: composeFile,
			})
		},
	}

	cmd.Flags().StringVar(&keepPorts, "keep-ports", "", "Comma-separated service names to keep host port bindings (e.g. db,redis)")
	cmd.Flags().StringVarP(&composeFile, "file", "f", "", "Path to docker-compose file (default: auto-detect or use saved config)")

	return cmd
}
```

- [ ] **Step 3: Build**

Run: `/usr/local/go/bin/go build -o build/caddy-atc ./cmd/caddy-atc`
Expected: Compiles successfully

- [ ] **Step 4: Verify help output includes -f flag**

Run: `./build/caddy-atc start --help`
Expected: Shows `-f, --file string   Path to docker-compose file (default: auto-detect or use saved config)`

- [ ] **Step 5: Run full test suite**

Run: `/usr/local/go/bin/go test ./... -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/start/start.go cmd/caddy-atc/main.go
git commit -m "feat: add -f flag to start command with config fallback"
```

---

### Task 6: Build final binary

**Files:**
- None (verification only)

- [ ] **Step 1: Full build and test**

Run: `/usr/local/go/bin/go build -o build/caddy-atc ./cmd/caddy-atc && /usr/local/go/bin/go test ./... -count=1 && /usr/local/go/bin/go vet ./...`
Expected: All pass, binary built

- [ ] **Step 2: Verify both commands show `-f` flag**

Run: `./build/caddy-atc adopt --help && ./build/caddy-atc start --help`
Expected: Both show `-f, --file` flag
