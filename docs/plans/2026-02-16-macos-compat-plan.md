# macOS Compatibility Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make caddy-atc compile and run correctly on macOS by replacing two Linux-specific code paths.

**Architecture:** Replace `/proc/<pid>/cmdline` with POSIX `ps` command for PID verification. Add `runtime.GOOS == "darwin"` branch in cert trust to print the macOS trust command.

**Tech Stack:** Go standard library (`os/exec`, `runtime`, `strconv`)

---

### Task 1: Replace `/proc/<pid>/cmdline` with POSIX `ps` command

**Files:**
- Modify: `cmd/caddy-atc/main.go:457-466`

**Context:** `isCaddyATCProcess(pid int) bool` currently reads `/proc/<pid>/cmdline` to verify a PID belongs to a caddy-atc process (guards against stale PID files). `/proc` doesn't exist on macOS. The POSIX-portable alternative is `ps -p <pid> -o comm=` which prints just the command name with no header.

**Step 1: Write the failing test**

Create `cmd/caddy-atc/main_test.go`:

```go
package main

import (
	"os"
	"testing"
)

func TestIsCaddyATCProcess_CurrentProcess(t *testing.T) {
	// Our own process won't be named "caddy-atc", but the function
	// should not panic or error on a valid PID
	pid := os.Getpid()
	// Should return false since test binary isn't named caddy-atc
	if isCaddyATCProcess(pid) {
		t.Error("isCaddyATCProcess(self) = true, want false for test binary")
	}
}

func TestIsCaddyATCProcess_NonExistentPID(t *testing.T) {
	// PID that almost certainly doesn't exist
	got := isCaddyATCProcess(9999999)
	if got {
		t.Error("isCaddyATCProcess(9999999) = true, want false for non-existent PID")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/caddy-atc/ -run TestIsCaddyATCProcess -v`
Expected: FAIL — the current implementation reads `/proc` which works on Linux, but the test for non-existent PID should validate behavior. On macOS this would fail to compile/run. The tests establish the contract.

**Step 3: Implement the fix**

Replace `isCaddyATCProcess` in `cmd/caddy-atc/main.go` (lines 457-466):

```go
// isCaddyATCProcess uses `ps` to verify the given PID is a caddy-atc process.
// Works on both Linux and macOS (POSIX).
func isCaddyATCProcess(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		// Process doesn't exist or ps failed — assume it's not ours
		return false
	}
	return strings.Contains(strings.TrimSpace(string(out)), "caddy-atc")
}
```

Add `"os/exec"` to the imports if not already present (it's not — current imports are: `context`, `fmt`, `io`, `log`, `os`, `os/signal`, `strconv`, `strings`, `syscall`, `text/tabwriter`, plus the project packages and cobra).

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/caddy-atc/ -run TestIsCaddyATCProcess -v`
Expected: PASS — both tests should pass. `CurrentProcess` returns false (test binary isn't "caddy-atc"). `NonExistentPID` returns false (ps exits non-zero).

**Step 5: Run all existing tests to check for regressions**

Run: `go test ./... -v`
Expected: All tests pass.

**Step 6: Commit**

```bash
git add cmd/caddy-atc/main.go cmd/caddy-atc/main_test.go
git commit -m "fix: replace /proc with POSIX ps for macOS compatibility"
```

---

### Task 2: Add macOS certificate trust instructions

**Files:**
- Modify: `internal/gateway/trust.go:65-76`

**Context:** `installCert(certPath string)` currently has branches for Linux and WSL. On macOS (`runtime.GOOS == "darwin"`), it falls through to a generic "manually install" message. Add a proper macOS branch that prints the `security add-trusted-cert` command.

**Step 1: Write the failing test**

Create `internal/gateway/trust_test.go`:

```go
package gateway

import (
	"runtime"
	"testing"
)

func TestInstallCertMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only test")
	}
	// On macOS, installCert should return nil (prints instructions, no error)
	err := installCert("/tmp/fake-cert.crt")
	if err != nil {
		t.Errorf("installCert() on macOS returned error: %v", err)
	}
}

func TestInstallCert_NonLinuxNonDarwin(t *testing.T) {
	// This test documents that non-Linux, non-Darwin platforms
	// get the generic fallback message without error.
	// We can only directly test on the current platform.
	if runtime.GOOS == "linux" {
		t.Skip("This test is for non-Linux platforms")
	}
	err := installCert("/tmp/fake-cert.crt")
	if err != nil {
		t.Errorf("installCert() returned error on %s: %v", runtime.GOOS, err)
	}
}
```

**Step 2: Run test to verify current behavior**

Run: `go test ./internal/gateway/ -run TestInstallCert -v`
Expected: On Linux, both tests skip. This establishes the test file compiles and skips correctly.

**Step 3: Implement the macOS branch**

Replace the `installCert` function in `internal/gateway/trust.go` (lines 65-76):

```go
func installCert(certPath string) error {
	switch runtime.GOOS {
	case "linux":
		if isWSL() {
			return installCertWSL(certPath)
		}
		return installCertLinux(certPath)
	case "darwin":
		return installCertDarwin(certPath)
	default:
		fmt.Printf("\nManually install the CA certificate:\n  %s\n", certPath)
		return nil
	}
}

func installCertDarwin(certPath string) error {
	fmt.Println()
	fmt.Println("To trust caddy-atc certificates in your browser, run:")
	fmt.Println()
	fmt.Printf("  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s\n", certPath)
	fmt.Println()
	fmt.Println("After importing, restart your browser for the change to take effect.")
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/gateway/ -run TestInstallCert -v`
Expected: Tests skip on Linux (as expected). No compilation errors.

**Step 5: Run all tests for regressions**

Run: `go test ./... -v`
Expected: All tests pass.

**Step 6: Build for macOS to verify cross-compilation**

Run: `GOOS=darwin GOARCH=arm64 go build ./cmd/caddy-atc`
Expected: Compiles successfully with exit code 0.

**Step 7: Commit**

```bash
git add internal/gateway/trust.go internal/gateway/trust_test.go
git commit -m "feat: add macOS certificate trust instructions"
```
