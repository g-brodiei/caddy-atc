# Design: macOS Compatibility

**Date:** 2026-02-16
**Status:** Approved

## Problem

caddy-atc has two Linux-specific code paths that prevent it from compiling or working correctly on macOS:

1. `isCaddyATCProcess()` reads `/proc/<pid>/cmdline` which doesn't exist on macOS
2. `installCert()` only handles Linux — macOS falls through to a generic "manually install" message

## Changes

### 1. PID verification — `isCaddyATCProcess` (`cmd/caddy-atc/main.go`)

Replace `/proc/<pid>/cmdline` with `ps -p <pid> -o comm=`. This is POSIX standard and works on both Linux and macOS. Returns the process command name, which we check for "caddy-atc".

**Before:** `os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))`
**After:** `exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()`

### 2. Certificate trust — `installCert` (`internal/gateway/trust.go`)

Add a `runtime.GOOS == "darwin"` branch that prints the macOS trust command for the user to copy-paste:

```
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain <cert-path>
```

Matches the pattern used for WSL Windows instructions (user runs the command themselves).

## What already works on macOS

- `syscall.Flock` — POSIX file locking
- `syscall.Exec` — process replacement in `caddy-atc start`
- `syscall.SIGINT/SIGTERM/Signal(0)` — signal handling
- Docker API via `client.FromEnv` — works with Docker Desktop
- File paths via `os.UserHomeDir()` + `filepath.Join`
- Bind-mount volume in gateway compose (`~/.caddy-atc/caddyfile`) — under `/Users/` which Docker Desktop shares by default
- Bridge networking and port binding — Docker Desktop handles transparently

## Files to modify

| File | Change |
|------|--------|
| `cmd/caddy-atc/main.go` | Replace `/proc` read with `ps` command in `isCaddyATCProcess` |
| `internal/gateway/trust.go` | Add `darwin` branch to `installCert` |
