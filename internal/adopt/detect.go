package adopt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeService represents a service from a docker-compose.yml file.
type ComposeService struct {
	Name   string
	Image  string
	Ports  []string
	IsHTTP bool
	Port   string // detected HTTP port
}

// Known HTTP server images.
var httpImages = map[string]string{
	"caddy":  "80",
	"nginx":  "80",
	"apache": "80",
	"httpd":  "80",
	"node":   "3000",
	"traefik": "80",
}

// Known non-HTTP images.
var nonHTTPImages = map[string]bool{
	"postgres":      true,
	"mysql":         true,
	"mariadb":       true,
	"mongo":         true,
	"redis":         true,
	"memcached":     true,
	"rabbitmq":      true,
	"elasticsearch": true,
	"kibana":        true,
	"zookeeper":     true,
	"kafka":         true,
	"mailhog":       true,
	"mailpit":       true,
	"minio":         true,
}

// Known HTTP ports.
var knownHTTPPorts = map[string]bool{
	"80": true, "443": true, "3000": true, "3001": true,
	"4000": true, "5000": true, "5173": true, "8000": true,
	"8080": true, "8443": true,
}

// Known non-HTTP ports.
var knownNonHTTPPorts = map[string]bool{
	"5432": true, "3306": true, "27017": true, "6379": true,
	"5672": true, "9200": true, "9300": true, "2181": true,
	"9092": true, "11211": true,
}

// composeFile is the minimal structure we parse from docker-compose.yml.
type composeFile struct {
	Services map[string]composeServiceDef `yaml:"services"`
}

type composeServiceDef struct {
	Image  string   `yaml:"image"`
	Build  any      `yaml:"build"`
	Ports  []string `yaml:"ports"`
	Expose []string `yaml:"expose"`
}

// ScanComposeFile reads a docker-compose.yml and detects HTTP services.
func ScanComposeFile(dir string) ([]ComposeService, error) {
	composePath := findComposeFile(dir)
	if composePath == "" {
		return nil, fmt.Errorf("no docker-compose.yml found in %s", dir)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", composePath, err)
	}

	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", composePath, err)
	}

	var services []ComposeService
	for name, svc := range cf.Services {
		cs := analyzeService(name, svc)
		services = append(services, cs)
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	return services, nil
}

func findComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func analyzeService(name string, svc composeServiceDef) ComposeService {
	cs := ComposeService{Name: name, Image: svc.Image}

	// Collect all ports (from ports and expose directives)
	for _, p := range svc.Ports {
		port := extractContainerPort(p)
		if port != "" {
			cs.Ports = append(cs.Ports, port)
		}
	}
	for _, p := range svc.Expose {
		cs.Ports = append(cs.Ports, p)
	}

	// Check by image name first
	imageName := extractImageBase(svc.Image)
	if nonHTTPImages[imageName] {
		cs.IsHTTP = false
		return cs
	}
	if port, ok := httpImages[imageName]; ok {
		cs.IsHTTP = true
		cs.Port = port
		return cs
	}

	// Check by service name
	if nonHTTPImages[name] {
		cs.IsHTTP = false
		return cs
	}

	// Check ports
	for _, port := range cs.Ports {
		if knownHTTPPorts[port] {
			cs.IsHTTP = true
			cs.Port = port
			return cs
		}
	}

	// If has a build context and ports, likely HTTP
	if svc.Build != nil && len(cs.Ports) > 0 {
		for _, port := range cs.Ports {
			if !knownNonHTTPPorts[port] {
				cs.IsHTTP = true
				cs.Port = port
				return cs
			}
		}
	}

	// If has ports that aren't known non-HTTP, assume HTTP
	for _, port := range cs.Ports {
		if !knownNonHTTPPorts[port] {
			cs.IsHTTP = true
			cs.Port = port
			return cs
		}
	}

	return cs
}

// extractContainerPort gets the container port from a port mapping like "8080:80" or "80".
func extractContainerPort(portSpec string) string {
	// Remove protocol suffix
	portSpec = strings.Split(portSpec, "/")[0]

	// Handle host:container or ip:host:container
	parts := strings.Split(portSpec, ":")
	containerPart := parts[len(parts)-1]

	// Handle ranges like "8000-8100"
	containerPart = strings.Split(containerPart, "-")[0]

	// Validate it's a number
	if _, err := strconv.Atoi(containerPart); err != nil {
		return ""
	}

	return containerPart
}

// extractImageBase gets the base image name (e.g., "caddy" from "caddy:2-alpine").
func extractImageBase(image string) string {
	// Remove registry prefix
	parts := strings.Split(image, "/")
	name := parts[len(parts)-1]
	// Remove tag
	name = strings.Split(name, ":")[0]
	return name
}
