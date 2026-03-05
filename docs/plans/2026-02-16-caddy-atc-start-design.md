# Design: `caddy-atc start` Command

**Date:** 2026-02-16
**Status:** Approved

## Problem

Running multiple Docker Compose projects simultaneously causes host port conflicts (port 80, 3000, 5432, 6379, etc.). Users must edit their project's docker-compose.yml to avoid conflicts, which defeats caddy-atc's goal of transparent routing.

## Solution

A `caddy-atc start` command that wraps any user command, transparently stripping all host port bindings from the project's compose files. Containers communicate only via the caddy-atc Docker network.

## Usage

```bash
# Simple - uses default `docker compose up -d`
caddy-atc start

# With custom command
caddy-atc start -- ./scripts/dev.sh

# With flags passed to the custom command
caddy-atc start -- ./scripts/dev.sh --build -w 4

# Explicit directory
caddy-atc start ~/project/investment -- ./scripts/dev.sh

# Keep host ports for specific services (e.g. for pgAdmin access)
caddy-atc start --keep-ports db,redis -- ./scripts/dev.sh
```

## How It Works

1. **Auto-adopt** - If the project isn't adopted, adopt it automatically (default hostname)
2. **Auto-start gateway** - If gateway/watcher aren't running, start them
3. **Read compose files** - Detect all files Docker Compose would load (base + overrides + COMPOSE_FILE)
4. **Generate stripped files** - Write `.caddy-atc-compose.yml` (and `.caddy-atc-compose.override.yml` if needed) with all `ports:` entries removed
5. **Set `COMPOSE_FILE` env var** - Point to the stripped file(s) so any `docker compose` call uses them
6. **Run user's command** - Execute with modified environment via `syscall.Exec()` (replaces process, signals forward naturally)
7. **Watcher handles routing** - Container events trigger network connection + route generation as usual

Default command (when no `--` is given): `docker compose up -d`

## Design Decisions

### Strip ALL ports, not just HTTP

Non-HTTP services (postgres, redis) also conflict across projects. Strip everything. Services communicate via Docker network by container name.

### Parse YAML ourselves, not `docker compose config`

`docker compose config` resolves `${VAR}` references to actual values. This breaks when:
- User scripts load env vars before calling docker compose (vars aren't set at strip time)
- Secrets get hardcoded into the stripped file

Instead: parse with Go `yaml.v3` Node API, walk tree, delete `ports:` nodes, write back. Variables stay as `${VAR}` literals.

### `syscall.Exec()` for user commands

Replaces the caddy-atc process with the user's command. Signals (Ctrl+C) go directly to the user's process. No orphaned containers. For the default `docker compose up -d` case, use `os/exec.Command` since it returns immediately.

### `--keep-ports` escape hatch

Some developers need host access to services via GUI tools (pgAdmin for postgres, redis-cli, etc.). `--keep-ports db,redis` preserves host port bindings for named services.

## Edge Cases Handled

| Edge Case | Solution |
|-----------|----------|
| Override files bypassed by COMPOSE_FILE | Detect and strip all auto-loaded files, include all in COMPOSE_FILE |
| `${VAR}` references resolved | YAML Node API parsing preserves variable references |
| Gateway not running | Auto-start gateway+watcher before running user command |
| Services needing host access | `--keep-ports svc1,svc2` flag |
| Ctrl+C / signal forwarding | `syscall.Exec()` replaces process, signals forward naturally |
| `expose:` directive | Only strip `ports:`, never touch `expose:` |
| Existing COMPOSE_FILE env var | Read all referenced files, strip each, rewrite env var |
| Compose project name mismatch | Existing `com.docker.compose.project` label handles this |

## New Commands

| Command | Description |
|---------|-------------|
| `caddy-atc start [dir] [--keep-ports svc,...] [-- cmd...]` | Strip ports and run command |
| `caddy-atc stop [dir]` | Stop project containers via stripped compose file |

## New Files

| File | Purpose |
|------|---------|
| `internal/start/start.go` | Core logic: detect files, strip, set env, exec |
| `internal/start/strip.go` | YAML manipulation: remove `ports:` from service nodes |
| `internal/start/strip_test.go` | Tests for port stripping (variable preservation, override handling, keep-ports) |
| `cmd/caddy-atc/main.go` | Add `start` and `stop` cobra commands |

## Stripped File Details

- Location: `.caddy-atc-compose.yml` in project root (next to original compose file)
- Regenerated each `caddy-atc start` invocation
- Users should add to `.gitignore`
- `caddy-atc stop` can optionally clean up the file

## Example: Investment Project

Original `docker-compose.yml` has ports for caddy-dev (3000), backend (8000), db (5432), redis (6379).

```bash
cd ~/project/investment
caddy-atc start -- ./scripts/dev.sh --build
```

1. Auto-adopt investment -> investment.localhost
2. Strip all ports from docker-compose.yml -> .caddy-atc-compose.yml
3. Set COMPOSE_FILE=.caddy-atc-compose.yml
4. exec ./scripts/dev.sh --build
5. dev.sh calls `docker compose --profile dev --profile workers up` which reads .caddy-atc-compose.yml
6. Watcher connects containers to caddy-atc network, creates routes:
   - investment.localhost -> caddy-dev:3000
   - backend.investment.localhost -> backend:8000
