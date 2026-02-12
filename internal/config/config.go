package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

// validName matches safe hostnames and container/service names:
// alphanumeric, dots, hyphens, underscores. Must start with alphanumeric.
var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateHostname checks that a hostname is safe to interpolate into a Caddyfile.
func ValidateHostname(s string) error {
	if s == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if len(s) > 253 {
		return fmt.Errorf("hostname too long: %d chars (max 253)", len(s))
	}
	if !validName.MatchString(s) {
		return fmt.Errorf("invalid hostname %q: must match [a-zA-Z0-9][a-zA-Z0-9._-]*", s)
	}
	return nil
}

// ValidateContainerName checks that a container name is safe for Caddyfile use.
func ValidateContainerName(s string) error {
	if s == "" {
		return fmt.Errorf("container name cannot be empty")
	}
	if !validName.MatchString(s) {
		return fmt.Errorf("invalid container name %q: must match [a-zA-Z0-9][a-zA-Z0-9._-]*", s)
	}
	return nil
}

// ValidatePort checks that a port string is a valid numeric port.
func ValidatePort(s string) error {
	if s == "" {
		return fmt.Errorf("port cannot be empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return fmt.Errorf("invalid port %q: must be numeric", s)
		}
	}
	return nil
}

// HomeDir returns the caddy-atc home directory (~/.caddy-atc).
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".caddy-atc")
	}
	return filepath.Join(home, ".caddy-atc")
}

// CaddyfileDir returns the directory where the Caddyfile is stored.
func CaddyfileDir() string {
	return filepath.Join(HomeDir(), "caddyfile")
}

// CaddyfilePath returns the full path to the generated Caddyfile.
func CaddyfilePath() string {
	return filepath.Join(CaddyfileDir(), "Caddyfile")
}

// ProjectsPath returns the path to the projects.yml file.
func ProjectsPath() string {
	return filepath.Join(HomeDir(), "projects.yml")
}

// LockPath returns the path to the config lock file.
func LockPath() string {
	return filepath.Join(HomeDir(), "projects.lock")
}

// LogPath returns the path to the watcher log file.
func LogPath() string {
	return filepath.Join(HomeDir(), "watcher.log")
}

// PidPath returns the path to the watcher PID file.
func PidPath() string {
	return filepath.Join(HomeDir(), "watcher.pid")
}

// ServiceConfig holds the hostname for a single service.
type ServiceConfig struct {
	Hostname string `yaml:"hostname"`
}

// ProjectConfig represents a single adopted project.
type ProjectConfig struct {
	Dir            string            `yaml:"dir"`
	ComposeProject string            `yaml:"compose_project"`
	Hostname       string            `yaml:"hostname"`
	Services       map[string]string `yaml:"services"`
}

// Config is the top-level config structure.
type Config struct {
	Projects map[string]*ProjectConfig `yaml:"projects"`
}

// EnsureHomeDir creates the caddy-atc home directory and subdirectories.
func EnsureHomeDir() error {
	dirs := []string{
		HomeDir(),
		CaddyfileDir(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}

// Load reads the projects config from disk.
func Load() (*Config, error) {
	path := ProjectsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Projects: make(map[string]*ProjectConfig)}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]*ProjectConfig)
	}
	return &cfg, nil
}

// Save writes the projects config to disk atomically (write tmp + rename).
func (c *Config) Save() error {
	if err := EnsureHomeDir(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return atomicWriteFile(ProjectsPath(), data, 0644)
}

// LoadAndModify loads the config under a file lock, calls the modifier function,
// and saves the result atomically. This prevents concurrent adopt/unadopt from
// overwriting each other's changes.
func LoadAndModify(fn func(*Config) error) error {
	if err := EnsureHomeDir(); err != nil {
		return err
	}

	lockFile, err := os.OpenFile(LockPath(), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer lockFile.Close()

	// Acquire exclusive lock
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring config lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	cfg, err := Load()
	if err != nil {
		return err
	}

	if err := fn(cfg); err != nil {
		return err
	}

	return cfg.Save()
}

// FindProjectByComposeProject looks up a project by its Docker Compose project name.
func (c *Config) FindProjectByComposeProject(composeName string) (string, *ProjectConfig) {
	for name, proj := range c.Projects {
		if proj.ComposeProject == composeName {
			return name, proj
		}
	}
	return "", nil
}

// atomicWriteFile writes data to a temp file then renames it to the target path.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// ResolveHostname returns the hostname for a service within a project.
func (p *ProjectConfig) ResolveHostname(serviceName string) string {
	if hostname, ok := p.Services[serviceName]; ok {
		return hostname
	}
	return serviceName + "." + p.Hostname
}

// FilterEnv returns os.Environ() with any existing key=... entries for the
// given key removed, preventing duplicates when appending.
func FilterEnv(key string) []string {
	prefix := strings.ToUpper(key) + "="
	var filtered []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), prefix) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
