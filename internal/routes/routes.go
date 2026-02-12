package routes

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/g-brodiei/caddy-atc/internal/config"
	"github.com/g-brodiei/caddy-atc/internal/gateway"
	"github.com/g-brodiei/caddy-atc/internal/watcher"
)

// ActiveRoute represents a currently active route for display.
type ActiveRoute struct {
	Hostname      string
	ContainerName string
	Port          string
	Project       string
	Service       string
	Status        string
}

// ListActive queries running containers and returns active routes.
func ListActive(ctx context.Context) ([]ActiveRoute, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var routes []ActiveRoute

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if name == gateway.ContainerName {
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

		info, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			continue
		}

		port := watcher.DetectHTTPPort(info)
		if port == "" {
			continue
		}

		hostname := projCfg.ResolveHostname(composeService)

		// Check if connected to caddy-atc network
		status := "routed"
		if info.NetworkSettings != nil {
			connected := false
			for netName := range info.NetworkSettings.Networks {
				if netName == gateway.NetworkName {
					connected = true
					break
				}
			}
			if !connected {
				status = "detected (not connected)"
			}
		}

		routes = append(routes, ActiveRoute{
			Hostname:      hostname,
			ContainerName: name,
			Port:          port,
			Project:       composeProject,
			Service:       composeService,
			Status:        status,
		})
	}

	return routes, nil
}
