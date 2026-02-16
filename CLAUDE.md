# CLAUDE.md - Development Guide for caddy-atc

## Build & Test

```bash
# Go is not on PATH in this environment
/usr/local/go/bin/go build -o build/caddy-atc ./cmd/caddy-atc
/usr/local/go/bin/go test ./... -count=1
/usr/local/go/bin/go vet ./...

# Or use Makefile (if go is on PATH)
make build
```

The installed binary is a symlink: `~/bin/caddy-atc -> ./build/caddy-atc`. Always rebuild after code changes.

## Project Structure

```
cmd/caddy-atc/main.go      CLI entrypoint (cobra commands)
internal/
  gateway/                  Docker container lifecycle
    gateway.go              Up/Down/Restart/IsRunning
    trust.go                CA certificate extraction & install
    compose.go              Embedded docker-compose.yml
    docker-compose.yml      Gateway container definition
  watcher/                  Docker event listener
    watcher.go              Event loop, route management, reload logic
    caddyfile.go            Caddyfile generation, ActiveRoutes, ReloadCaddy
    detect.go               Runtime HTTP port detection (from container inspect)
  adopt/                    Project adoption
    adopt.go                Adopt/unadopt workflows
    detect.go               Compose file + Dockerfile scanning for HTTP services
  start/                    Port-conflict-free project launching
    strip.go                YAML port stripping (yaml.v3 Node API)
    compose.go              Compose file detection, stripped file generation
    start.go                Start/stop orchestration (auto-adopt, exec)
  config/                   Configuration
    config.go               Paths, validation, config load/save, file locking
  routes/                   Status queries
    routes.go               List active routes for display
```

## Key Design Decisions

- **Caddyfile generation**: Routes are written to `~/.caddy-atc/caddyfile/Caddyfile`, mounted read-only into the gateway container. Reload via `docker exec caddy-atc caddy reload`.
- **Atomic writes**: Both Caddyfile and config use temp file + rename to prevent partial writes.
- **Validation**: All values interpolated into Caddyfiles are validated against `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` to prevent injection.
- **Docker exec recovery**: `reloadRoutes` uses try-first-then-recover (not check-then-act) to handle zombie containers and TOCTOU races.
- **Port detection at adopt time**: Scans both compose `ports:`/`expose:` directives AND Dockerfile `EXPOSE` directives for services with `build:` context.

## Common Pitfalls

- **WSL2 zombie containers**: After sleep/hibernate, Docker API may report containers as running while `docker exec` fails. The `gateway.Restart()` function handles this via `ContainerRestart` API.
- **Binary not rebuilt**: The most common reason a fix "doesn't work" is forgetting to rebuild `build/caddy-atc` after code changes.
- **Docker name filter is substring**: `ContainerList` with `name=X` matches substrings. Always do exact match on returned names.
