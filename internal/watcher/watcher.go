package watcher

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/g-brodiei/caddy-atc/internal/config"
	"github.com/g-brodiei/caddy-atc/internal/gateway"
)

// Watcher monitors Docker events and manages routes.
type Watcher struct {
	cli    *client.Client
	routes *ActiveRoutes
	logger *log.Logger
}

// New creates a new Watcher.
func New(logger *log.Logger) (*Watcher, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}

	return &Watcher{
		cli:    cli,
		routes: NewActiveRoutes(),
		logger: logger,
	}, nil
}

// Close releases the Docker client.
func (w *Watcher) Close() {
	if w.cli != nil {
		w.cli.Close()
	}
}

// Run starts the watcher: scans existing containers, then listens for events.
func (w *Watcher) Run(ctx context.Context) error {
	w.logger.Println("Starting watcher...")

	// Scan existing containers on startup
	if err := w.scanExisting(ctx); err != nil {
		w.logger.Printf("Warning: failed to scan existing containers: %v", err)
	}

	// Listen for Docker events
	eventFilter := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "stop"),
		filters.Arg("event", "die"),
	)

	msgCh, errCh := w.cli.Events(ctx, events.ListOptions{Filters: eventFilter})

	w.logger.Println("Watching for container events...")

	for {
		select {
		case <-ctx.Done():
			w.logger.Println("Watcher stopping.")
			return nil
		case err := <-errCh:
			if err != nil {
				return fmt.Errorf("Docker event error: %w", err)
			}
		case msg := <-msgCh:
			w.handleEvent(ctx, msg)
		}
	}
}

// Routes returns the active routes (for status/routes commands).
func (w *Watcher) Routes() *ActiveRoutes {
	return w.routes
}

func (w *Watcher) handleEvent(ctx context.Context, msg events.Message) {
	containerID := msg.Actor.ID
	containerName := msg.Actor.Attributes["name"]

	// Skip our own caddy container
	if containerName == gateway.ContainerName {
		return
	}

	switch msg.Action {
	case "start":
		w.logger.Printf("Container started: %s (%s)", containerName, shortID(containerID))
		w.handleContainerStart(ctx, containerID)
	case "stop", "die":
		w.logger.Printf("Container stopped: %s (%s)", containerName, shortID(containerID))
		w.handleContainerStop(ctx, containerID)
	}
}

func (w *Watcher) handleContainerStart(ctx context.Context, containerID string) {
	cfg, err := config.Load()
	if err != nil {
		w.logger.Printf("Error loading config: %v", err)
		return
	}

	info, err := w.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		w.logger.Printf("Error inspecting container %s: %v", shortID(containerID), err)
		return
	}

	// Get compose project and service from labels
	composeProject := info.Config.Labels["com.docker.compose.project"]
	composeService := info.Config.Labels["com.docker.compose.service"]

	if composeProject == "" {
		w.logger.Printf("Container %s has no compose project label, skipping", info.Name)
		return
	}

	// Look up in adopted projects
	_, projCfg := cfg.FindProjectByComposeProject(composeProject)
	if projCfg == nil {
		return // not adopted, ignore silently
	}

	// Detect HTTP port
	port := DetectHTTPPort(info)
	if port == "" {
		w.logger.Printf("No HTTP port detected for %s/%s, skipping (hint: add EXPOSE <port> to the Dockerfile or label caddy-atc.port=<port> in docker-compose.yml)", composeProject, composeService)
		return
	}

	// Determine hostname
	hostname := projCfg.ResolveHostname(composeService)

	// Validate before adding route
	if err := config.ValidateHostname(hostname); err != nil {
		w.logger.Printf("Invalid hostname for %s/%s: %v", composeProject, composeService, err)
		return
	}

	// Connect container to caddy-atc network
	containerName := strings.TrimPrefix(info.Name, "/")
	if err := config.ValidateContainerName(containerName); err != nil {
		w.logger.Printf("Invalid container name %q: %v", containerName, err)
		return
	}

	if err := w.connectToNetwork(ctx, containerID); err != nil {
		w.logger.Printf("Error connecting %s to network: %v", containerName, err)
		return
	}

	// Add route
	route := &Route{
		Hostname:      hostname,
		ContainerName: containerName,
		Port:          port,
		Project:       composeProject,
		Service:       composeService,
	}
	w.routes.Add(containerID, route)

	w.logger.Printf("Route added: %s -> %s:%s", hostname, containerName, port)

	// Regenerate Caddyfile and reload
	if err := w.reloadRoutes(ctx); err != nil {
		w.logger.Printf("Error reloading routes: %v", err)
	}
}

func (w *Watcher) handleContainerStop(ctx context.Context, containerID string) {
	route, ok := w.routes.Get(containerID)
	if !ok {
		return // not a routed container
	}

	w.logger.Printf("Route removed: %s -> %s:%s", route.Hostname, route.ContainerName, route.Port)
	w.routes.Remove(containerID)

	if err := w.reloadRoutes(ctx); err != nil {
		w.logger.Printf("Error reloading routes: %v", err)
	}
}

func (w *Watcher) scanExisting(ctx context.Context) error {
	w.logger.Println("Scanning existing containers...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	containers, err := w.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}

	for _, c := range containers {
		// Skip the gateway container
		if isGatewayContainer(c.Names) {
			continue
		}

		composeProject := c.Labels["com.docker.compose.project"]
		composeService := c.Labels["com.docker.compose.service"]
		if composeProject == "" {
			continue
		}

		_, projCfg := cfg.FindProjectByComposeProject(composeProject)
		if projCfg == nil {
			continue
		}

		info, err := w.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			w.logger.Printf("Error inspecting container %s: %v", shortID(c.ID), err)
			continue
		}

		port := DetectHTTPPort(info)
		if port == "" {
			w.logger.Printf("No HTTP port detected for %s/%s, skipping (hint: add EXPOSE <port> to the Dockerfile or label caddy-atc.port=<port> in docker-compose.yml)", composeProject, composeService)
			continue
		}

		hostname := projCfg.ResolveHostname(composeService)
		containerName := strings.TrimPrefix(info.Name, "/")

		// Validate before adding route
		if err := config.ValidateHostname(hostname); err != nil {
			w.logger.Printf("Invalid hostname for %s/%s: %v, skipping", composeProject, composeService, err)
			continue
		}
		if err := config.ValidateContainerName(containerName); err != nil {
			w.logger.Printf("Invalid container name %q: %v, skipping", containerName, err)
			continue
		}

		// Connect to network
		if err := w.connectToNetwork(ctx, c.ID); err != nil {
			w.logger.Printf("Error connecting %s to network: %v", containerName, err)
			continue
		}

		route := &Route{
			Hostname:      hostname,
			ContainerName: containerName,
			Port:          port,
			Project:       composeProject,
			Service:       composeService,
		}
		w.routes.Add(c.ID, route)
		w.logger.Printf("Existing route: %s -> %s:%s", hostname, containerName, port)
	}

	if w.routes.Len() > 0 {
		if err := w.reloadRoutes(ctx); err != nil {
			return fmt.Errorf("reloading routes: %w", err)
		}
	}

	w.logger.Printf("Found %d active routes", w.routes.Len())
	return nil
}

func (w *Watcher) connectToNetwork(ctx context.Context, containerID string) error {
	// Check if already connected
	info, err := w.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	if info.NetworkSettings != nil {
		for name := range info.NetworkSettings.Networks {
			if name == gateway.NetworkName {
				return nil // already connected
			}
		}
	}

	return w.cli.NetworkConnect(ctx, gateway.NetworkName, containerID, &network.EndpointSettings{})
}

func (w *Watcher) reloadRoutes(ctx context.Context) error {
	if err := WriteCaddyfile(w.routes); err != nil {
		return fmt.Errorf("writing Caddyfile: %w", err)
	}

	// Ensure gateway container is running before attempting reload
	running, err := gateway.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("checking gateway: %w", err)
	}
	if !running {
		w.logger.Println("Gateway container not running, starting it...")
		if err := gateway.Up(ctx); err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
		// Brief pause for Caddy to finish initializing inside the container
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err := ReloadCaddy(ctx); err != nil {
		return fmt.Errorf("reloading Caddy: %w", err)
	}
	return nil
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func isGatewayContainer(names []string) bool {
	for _, name := range names {
		if strings.TrimPrefix(name, "/") == gateway.ContainerName {
			return true
		}
	}
	return false
}
