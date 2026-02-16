# caddy-atc

Local development gateway that eliminates Docker port conflicts. Routes HTTP traffic to your project containers by hostname through a single Caddy reverse proxy.

Instead of juggling port numbers across projects (`localhost:3000`, `localhost:3001`, `localhost:8080`...), each project gets a clean hostname:

```
backend.myproject.localhost
frontend.myproject.localhost
api.other-project.localhost
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
| `caddy-atc logs [-f]` | Show (or follow) watcher logs |

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
- Go 1.24+ (for building)
- Linux or WSL2
