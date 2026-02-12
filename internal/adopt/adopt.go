package adopt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g-brodiei/caddy-atc/internal/config"
)

// Result holds the result of an adopt operation.
type Result struct {
	ProjectName     string
	Dir             string
	Hostname        string
	HTTPServices    []ComposeService
	SkippedServices []ComposeService
}

// Adopt scans a project directory and registers it in the config.
func Adopt(dir string, hostname string, dryRun bool) (*Result, error) {
	// Resolve absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Verify directory exists
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absDir)
	}

	// Determine project name from directory
	projectName := filepath.Base(absDir)

	// Default hostname
	if hostname == "" {
		hostname = projectName + ".localhost"
	}

	// Validate hostname
	if err := config.ValidateHostname(hostname); err != nil {
		return nil, fmt.Errorf("invalid hostname: %w", err)
	}

	// Scan compose file
	services, err := ScanComposeFile(absDir)
	if err != nil {
		return nil, err
	}

	// Determine compose project name (Docker Compose uses directory name by default)
	composeProject := projectName

	// Separate HTTP and non-HTTP services
	var httpServices, skippedServices []ComposeService
	for _, svc := range services {
		if svc.IsHTTP {
			httpServices = append(httpServices, svc)
		} else {
			skippedServices = append(skippedServices, svc)
		}
	}

	if len(httpServices) == 0 {
		return nil, fmt.Errorf("no HTTP services detected in %s", absDir)
	}

	// Assign hostnames
	svcHostnames := assignHostnames(httpServices, hostname)

	// Validate all generated hostnames
	for svc, h := range svcHostnames {
		if err := config.ValidateHostname(h); err != nil {
			return nil, fmt.Errorf("invalid hostname for service %q: %w", svc, err)
		}
	}

	result := &Result{
		ProjectName:     projectName,
		Dir:             absDir,
		Hostname:        hostname,
		HTTPServices:    httpServices,
		SkippedServices: skippedServices,
	}

	if dryRun {
		return result, nil
	}

	// Save to config with file locking to prevent TOCTOU races
	err = config.LoadAndModify(func(cfg *config.Config) error {
		cfg.Projects[projectName] = &config.ProjectConfig{
			Dir:            absDir,
			ComposeProject: composeProject,
			Hostname:       hostname,
			Services:       svcHostnames,
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("saving config: %w", err)
	}

	return result, nil
}

// Unadopt removes a project from the config.
func Unadopt(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	projectName := filepath.Base(absDir)

	// Use file locking to prevent TOCTOU races
	return config.LoadAndModify(func(cfg *config.Config) error {
		if _, ok := cfg.Projects[projectName]; !ok {
			return fmt.Errorf("project %q is not adopted", projectName)
		}
		delete(cfg.Projects, projectName)
		return nil
	})
}

func assignHostnames(services []ComposeService, baseHostname string) map[string]string {
	hostnames := make(map[string]string)

	// Find the "primary" service - one that maps to base hostname
	primaryIdx := FindPrimaryService(services)

	for i, svc := range services {
		if i == primaryIdx {
			hostnames[svc.Name] = baseHostname
		} else {
			prefix := svc.Name
			hostnames[svc.Name] = prefix + "." + baseHostname
		}
	}

	return hostnames
}

// FindPrimaryService identifies which service should get the base hostname.
// Priority: caddy > nginx > httpd/apache > service named "web" > first service on port 80.
func FindPrimaryService(services []ComposeService) int {
	primaryImages := []string{"caddy", "nginx", "httpd", "apache"}
	primaryNames := []string{"web", "app", "caddy", "nginx"}

	// Check by image
	for _, img := range primaryImages {
		for i, svc := range services {
			if strings.Contains(extractImageBase(svc.Image), img) {
				return i
			}
		}
	}

	// Check by service name
	for _, name := range primaryNames {
		for i, svc := range services {
			if svc.Name == name {
				return i
			}
		}
	}

	// Check for port 80
	for i, svc := range services {
		if svc.Port == "80" {
			return i
		}
	}

	return 0
}
