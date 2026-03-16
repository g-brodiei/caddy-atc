# Wildcard Hostname Support Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow projects to use wildcard hostnames (e.g., `*.curate.localhost`) so caddy-atc can route all subdomains to a project's internal reverse proxy.

**Architecture:** Extend hostname validation to accept `*.` prefix. Update `ResolveHostname` and `assignHostnames` to strip the `*.` prefix when building subservice hostnames (e.g., `*.curate.localhost` primary, `client.curate.localhost` subservice). The Caddyfile generator already handles this since it just writes the hostname string — Caddy natively supports wildcard site blocks.

**Tech Stack:** Go, yaml.v3, existing test infrastructure

---

## File Structure

- Modify: `internal/config/config.go` — `ValidateHostname` + `ResolveHostname`
- Modify: `internal/config/config_test.go` — wildcard test cases
- Modify: `internal/adopt/adopt.go` — `assignHostnames`
- Modify: `internal/adopt/adopt_test.go` — wildcard test cases
- Modify: `internal/watcher/caddyfile_test.go` — wildcard Caddyfile generation test

---

### Task 1: ValidateHostname — accept wildcard prefix

**Files:**
- Modify: `internal/config/config.go:22-33`
- Modify: `internal/config/config_test.go:11-40`

- [ ] **Step 1: Add failing tests for wildcard hostnames**

Add these cases to `TestValidateHostname` in `internal/config/config_test.go`:

```go
{"valid wildcard", "*.curate.localhost", false},
{"valid wildcard simple", "*.localhost", false},
{"wildcard only star", "*", true},
{"wildcard no dot", "*localhost", true},
{"wildcard double star", "**.localhost", true},
{"wildcard mid-string", "foo.*.localhost", true},
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestValidateHostname -v -count=1`
Expected: FAIL — `*.curate.localhost` and `*.localhost` rejected by current regex

- [ ] **Step 3: Update ValidateHostname to accept `*.` prefix**

In `internal/config/config.go`, update `ValidateHostname`:

```go
func ValidateHostname(s string) error {
	if s == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(s) > 253 {
		return fmt.Errorf("hostname too long: %d chars (max 253)", len(s))
	}
	check := s
	if strings.HasPrefix(s, "*.") {
		check = s[2:]
	}
	if !validName.MatchString(check) {
		return fmt.Errorf("invalid hostname %q: must match [a-zA-Z0-9][a-zA-Z0-9._-]* (optionally prefixed with *.)", s)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestValidateHostname -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: allow wildcard hostnames in ValidateHostname"
```

---

### Task 2: ResolveHostname — strip wildcard for subservices

**Files:**
- Modify: `internal/config/config.go:249-255`
- Modify: `internal/config/config_test.go:90-117`

- [ ] **Step 1: Add failing test for wildcard ResolveHostname**

Add a new test case in `TestResolveHostname` in `internal/config/config_test.go`. Add a second subtest with a wildcard project config:

```go
t.Run("wildcard hostname", func(t *testing.T) {
	wp := &ProjectConfig{
		Hostname: "*.curate.localhost",
		Services: map[string]string{
			"caddy": "*.curate.localhost",
		},
	}

	tests := []struct {
		name        string
		serviceName string
		want        string
	}{
		{"explicit wildcard mapping", "caddy", "*.curate.localhost"},
		{"auto-generated strips wildcard", "client", "client.curate.localhost"},
		{"auto-generated strips wildcard 2", "server", "server.curate.localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wp.ResolveHostname(tt.serviceName)
			if got != tt.want {
				t.Errorf("ResolveHostname(%q) = %q, want %q", tt.serviceName, got, tt.want)
			}
		})
	}
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestResolveHostname -v -count=1`
Expected: FAIL — `client` resolves to `client.*.curate.localhost` instead of `client.curate.localhost`

- [ ] **Step 3: Update ResolveHostname to strip wildcard prefix**

In `internal/config/config.go`, update `ResolveHostname`:

```go
func (p *ProjectConfig) ResolveHostname(serviceName string) string {
	if hostname, ok := p.Services[serviceName]; ok {
		return hostname
	}
	base := p.Hostname
	if strings.HasPrefix(base, "*.") {
		base = base[2:]
	}
	return serviceName + "." + base
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/usr/local/go/bin/go test ./internal/config/ -run TestResolveHostname -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: strip wildcard prefix in ResolveHostname for subservices"
```

---

### Task 3: assignHostnames — strip wildcard for non-primary services

**Files:**
- Modify: `internal/adopt/adopt.go:130-146`
- Modify: `internal/adopt/adopt_test.go:89-123`

- [ ] **Step 1: Add failing test for wildcard assignHostnames**

Add to `TestAssignHostnames` in `internal/adopt/adopt_test.go`:

```go
t.Run("wildcard primary, others get stripped prefix", func(t *testing.T) {
	services := []ComposeService{
		{Name: "caddy", Image: "caddy:2", Port: "80", IsHTTP: true},
		{Name: "client", Image: "node:18", Port: "3000", IsHTTP: true},
		{Name: "server", Image: "node:18", Port: "3001", IsHTTP: true},
	}
	hostnames := assignHostnames(services, "*.curate.localhost")
	if hostnames["caddy"] != "*.curate.localhost" {
		t.Errorf("caddy hostname = %q, want %q", hostnames["caddy"], "*.curate.localhost")
	}
	if hostnames["client"] != "client.curate.localhost" {
		t.Errorf("client hostname = %q, want %q", hostnames["client"], "client.curate.localhost")
	}
	if hostnames["server"] != "server.curate.localhost" {
		t.Errorf("server hostname = %q, want %q", hostnames["server"], "server.curate.localhost")
	}
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/usr/local/go/bin/go test ./internal/adopt/ -run TestAssignHostnames -v -count=1`
Expected: FAIL — `client` gets `client.*.curate.localhost`

- [ ] **Step 3: Update assignHostnames to strip wildcard prefix**

In `internal/adopt/adopt.go`, update `assignHostnames`:

```go
func assignHostnames(services []ComposeService, baseHostname string) map[string]string {
	hostnames := make(map[string]string)

	// Find the "primary" service - one that maps to base hostname
	primaryIdx := FindPrimaryService(services)

	// For non-primary services, strip wildcard prefix if present
	subBase := baseHostname
	if strings.HasPrefix(subBase, "*.") {
		subBase = subBase[2:]
	}

	for i, svc := range services {
		if i == primaryIdx {
			hostnames[svc.Name] = baseHostname
		} else {
			hostnames[svc.Name] = svc.Name + "." + subBase
		}
	}

	return hostnames
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/usr/local/go/bin/go test ./internal/adopt/ -run TestAssignHostnames -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adopt/adopt.go internal/adopt/adopt_test.go
git commit -m "feat: strip wildcard prefix in assignHostnames for subservices"
```

---

### Task 4: Caddyfile generation — verify wildcard hostnames work

**Files:**
- Modify: `internal/watcher/caddyfile_test.go`

- [ ] **Step 1: Add test for wildcard hostname in Caddyfile**

Add to `internal/watcher/caddyfile_test.go`:

```go
func TestGenerateCaddyfile_WildcardHostname(t *testing.T) {
	routes := NewActiveRoutes()
	routes.Add("c1", &Route{
		Hostname:      "*.curate.localhost",
		ContainerName: "monolith-caddy-1",
		Port:          "80",
	})
	routes.Add("c2", &Route{
		Hostname:      "client.curate.localhost",
		ContainerName: "monolith-client-1",
		Port:          "3000",
	})

	got, err := GenerateCaddyfile(routes)
	if err != nil {
		t.Fatalf("GenerateCaddyfile() error = %v", err)
	}

	if !strings.Contains(got, "*.curate.localhost {") {
		t.Error("expected wildcard hostname block")
	}
	if !strings.Contains(got, "client.curate.localhost {") {
		t.Error("expected specific hostname block")
	}
	if !strings.Contains(got, "reverse_proxy monolith-caddy-1:80") {
		t.Error("expected reverse_proxy for wildcard")
	}
	if !strings.Contains(got, "reverse_proxy monolith-client-1:3000") {
		t.Error("expected reverse_proxy for client")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `/usr/local/go/bin/go test ./internal/watcher/ -run TestGenerateCaddyfile_WildcardHostname -v -count=1`
Expected: PASS (ValidateHostname already updated in Task 1)

- [ ] **Step 3: Commit**

```bash
git add internal/watcher/caddyfile_test.go
git commit -m "test: verify wildcard hostname in Caddyfile generation"
```

---

### Task 5: Full test suite verification

- [ ] **Step 1: Run all tests**

Run: `/usr/local/go/bin/go test ./... -count=1`
Expected: All PASS

- [ ] **Step 2: Run vet**

Run: `/usr/local/go/bin/go vet ./...`
Expected: No issues

- [ ] **Step 3: Build**

Run: `PATH="/usr/local/go/bin:$PATH" make build`
Expected: Clean build
