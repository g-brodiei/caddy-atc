package start

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// atomicWriteFile writes to a temp file then renames to prevent partial writes.
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

const strippedPrefix = ".caddy-atc-compose"

// DetectComposeFiles finds which compose files Docker Compose would load
// for the given project directory. If composeFile is provided (non-empty),
// uses that file and looks for overrides. Otherwise, checks COMPOSE_FILE env var
// first, then falls back to standard file detection (with override auto-loading).
func DetectComposeFiles(dir string, composeFile string) ([]string, error) {
	if composeFile != "" {
		var base string
		if filepath.IsAbs(composeFile) {
			base = composeFile
		} else {
			base = filepath.Join(dir, composeFile)
		}
		if _, err := os.Stat(base); err != nil {
			return nil, fmt.Errorf("compose file not found: %s", base)
		}
		files := []string{base}
		override := findOverrideFile(filepath.Dir(base), base)
		if override != "" {
			files = append(files, override)
		}
		return files, nil
	}

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
// If regenerate is false and the stripped file already exists, it is reused as-is.
// Returns the paths to the stripped files in the same order.
func GenerateStrippedFiles(originals []string, keepPorts []string, regenerate bool) ([]string, error) {
	var stripped []string

	for i, orig := range originals {
		dir := filepath.Dir(orig)
		name := strippedFilename(i, len(originals))
		outPath := filepath.Join(dir, name)

		// Skip generation if file exists and regenerate is not requested
		if !regenerate {
			if _, err := os.Stat(outPath); err == nil {
				stripped = append(stripped, outPath)
				continue
			}
		}

		data, err := os.ReadFile(orig)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", orig, err)
		}

		out, err := StripPorts(data, keepPorts)
		if err != nil {
			return nil, fmt.Errorf("stripping ports from %s: %w", orig, err)
		}

		if err := atomicWriteFile(outPath, out, 0644); err != nil {
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
