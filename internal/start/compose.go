package start

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const strippedPrefix = ".caddy-atc-compose"

// DetectComposeFiles finds which compose files Docker Compose would load
// for the given project directory. Checks COMPOSE_FILE env var first, then
// falls back to standard file detection (with override auto-loading).
func DetectComposeFiles(dir string) ([]string, error) {
	if envVal := os.Getenv("COMPOSE_FILE"); envVal != "" {
		sep := ":"
		parts := strings.Split(envVal, sep)
		var files []string
		for _, p := range parts {
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("COMPOSE_FILE references missing file: %s", p)
			}
			files = append(files, p)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("COMPOSE_FILE is set but contains no valid files")
		}
		return files, nil
	}

	base := findBaseComposeFile(dir)
	if base == "" {
		return nil, fmt.Errorf("no docker-compose.yml or compose.yml found in %s", dir)
	}

	files := []string{base}

	override := findOverrideFile(dir, base)
	if override != "" {
		files = append(files, override)
	}

	return files, nil
}

// GenerateStrippedFiles creates port-stripped copies of the given compose files.
// Returns the paths to the stripped files in the same order.
func GenerateStrippedFiles(originals []string, keepPorts []string) ([]string, error) {
	var stripped []string

	for i, orig := range originals {
		data, err := os.ReadFile(orig)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", orig, err)
		}

		out, err := StripPorts(data, keepPorts)
		if err != nil {
			return nil, fmt.Errorf("stripping ports from %s: %w", orig, err)
		}

		dir := filepath.Dir(orig)
		name := strippedFilename(i, len(originals))
		outPath := filepath.Join(dir, name)

		if err := os.WriteFile(outPath, out, 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", outPath, err)
		}

		stripped = append(stripped, outPath)
	}

	return stripped, nil
}

// BuildComposeFileEnv builds the COMPOSE_FILE env var value from file paths.
func BuildComposeFileEnv(files []string) string {
	return strings.Join(files, ":")
}

func findBaseComposeFile(dir string) string {
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

func findOverrideFile(dir string, base string) string {
	baseName := filepath.Base(base)
	ext := filepath.Ext(baseName)
	nameNoExt := strings.TrimSuffix(baseName, ext)

	override := nameNoExt + ".override" + ext
	p := filepath.Join(dir, override)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func strippedFilename(index int, total int) string {
	if total == 1 {
		return strippedPrefix + ".yml"
	}
	if index == 0 {
		return strippedPrefix + ".yml"
	}
	return strippedPrefix + ".override.yml"
}
