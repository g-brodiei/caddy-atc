# caddy-atc

[![CI](https://github.com/g-brodiei/caddy-atc/actions/workflows/ci.yml/badge.svg)](https://github.com/g-brodiei/caddy-atc/actions/workflows/ci.yml)
[![Release](https://github.com/g-brodiei/caddy-atc/actions/workflows/release.yml/badge.svg)](https://github.com/g-brodiei/caddy-atc/actions/workflows/release.yml)
[![GitHub Release](https://img.shields.io/github/v/release/g-brodiei/caddy-atc)](https://github.com/g-brodiei/caddy-atc/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Local development gateway that eliminates Docker port conflicts. Routes HTTP traffic to your project containers by hostname through a single Caddy reverse proxy.

Instead of juggling port numbers across projects (`localhost:3000`, `localhost:3001`, `localhost:8080`...), each project gets a clean hostname:

```
backend.myproject.localhost
frontend.myproject.localhost
api.other-project.localhost
```

## Installation

### Quick install (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/g-brodiei/caddy-atc/main/install.sh | sh
```

Pin a specific version:

```bash
VERSION=0.1.0 curl -fsSL https://raw.githubusercontent.com/g-brodiei/caddy-atc/main/install.sh | sh
```

Pre-built binaries are available on the [Releases page](https://github.com/g-brodiei/caddy-atc/releases).

### From source

Requires Go 1.24+:

```bash
git clone https://github.com/g-brodiei/caddy-atc.git
cd caddy-atc
make build
# Binary is at ./build/caddy-atc
```

## How It Works

1. A single Caddy container (`caddy-atc`) binds ports 80/443 on the host
2. A watcher monitors Docker events for container start/stop
3. When an adopted project's container starts, caddy-atc automatically:
   - Connects it to the `caddy-atc` Docker network
   - Detects its HTTP port
   - Generates a Caddyfile with reverse proxy rules
   - Reloads Caddy with the new config
4. HTTPS with auto-generated local certificates via Caddy's internal CA

## Quick Start

```bash
# Build
make build

# Start the gateway and watcher
caddy-atc up

# Adopt a project (from the project directory with docker-compose.yml)
cd ~/project/my-app
caddy-atc adopt

# Start your project normally
docker compose up -d

# Your services are now available at:
#   https://my-app.localhost        (primary service)
#   https://api.my-app.localhost    (other services)
```

## Commands

| Command | Description |
|---------|-------------|
| `caddy-atc up` | Start the gateway container and watcher |
| `caddy-atc down` | Stop the gateway and watcher |
| `caddy-atc adopt [dir]` | Register a project for automatic routing |
| `caddy-atc unadopt [dir]` | Remove a project from routing |
| `caddy-atc status` | Show gateway health and active routes |
| `caddy-atc routes` | List all active routes |
| `caddy-atc trust` | Install Caddy's root CA in system trust store |
| `caddy-atc start [dir] [-- cmd]` | Start project with ports stripped |
| `caddy-atc stop [dir]` | Stop project and clean up stripped files |
| `caddy-atc logs [-f]` | Show (or follow) watcher logs |

### Starting Projects

Use `caddy-atc start` to run multiple projects simultaneously without port conflicts:

```bash
caddy-atc start                              # docker compose up -d (default)
caddy-atc start -- ./scripts/dev.sh          # custom start script
caddy-atc start --keep-ports db,redis        # keep host ports for specific services
caddy-atc stop                               # stop and clean up
```

This strips all host port bindings from the compose file and sets `COMPOSE_FILE` so any `docker compose` calls in your script use the stripped version. Add `.caddy-atc-compose*.yml` to your `.gitignore`.

### Adopt Options

```bash
caddy-atc adopt                    # Adopt current directory
caddy-atc adopt ~/project/my-app   # Adopt specific directory
caddy-atc adopt --hostname myapp.localhost  # Override base hostname
caddy-atc adopt --dry-run          # Preview without saving
```

## HTTP Service Detection

caddy-atc detects HTTP services from your docker-compose.yml through:

1. **Image name** - Known HTTP servers (caddy, nginx, node, etc.)
2. **Port mappings** - `ports:` and `expose:` in compose
3. **Dockerfile EXPOSE** - Scans referenced Dockerfiles for `EXPOSE` directives
4. **Known ports** - 80, 443, 3000, 5173, 8000, 8080, etc.

Non-HTTP services (postgres, redis, etc.) are automatically skipped.

## Hostname Resolution

- The **primary service** (detected by image/name heuristics) gets the base hostname: `myproject.localhost`
- Other services are prefixed: `api.myproject.localhost`, `worker.myproject.localhost`
- Multiple containers for the same service (replicas) share a hostname with Caddy load balancing

## HTTPS / Trust

caddy-atc uses Caddy's internal CA to issue certificates for `*.localhost` domains. To avoid browser certificate warnings:

```bash
caddy-atc trust
```

On WSL2, this installs the CA cert in the Linux trust store and provides instructions for the Windows certificate store (required for Chrome/Edge).

## Configuration

Config is stored in `~/.caddy-atc/`:

```
~/.caddy-atc/
  projects.yml          # Adopted projects
  caddyfile/Caddyfile   # Auto-generated (do not edit)
  watcher.log           # Watcher logs
  watcher.pid           # Watcher PID file
```

## Requirements

- Docker with Compose V2
- Linux, macOS, or WSL2
- Go 1.24+ (only needed for building from source)
