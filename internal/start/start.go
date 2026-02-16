package start

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/g-brodiei/caddy-atc/internal/adopt"
	"github.com/g-brodiei/caddy-atc/internal/config"
	"github.com/g-brodiei/caddy-atc/internal/gateway"
)

// Options configures the start command.
type Options struct {
	Dir       string   // Project directory (resolved to absolute)
	KeepPorts []string // Service names whose ports should be kept
	Command   []string // User command to run (nil = docker compose up -d)
}

// Run executes the start workflow: auto-adopt, ensure gateway, strip ports, exec command.
func Run(ctx context.Context, opts Options) error {
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}

	// 1. Auto-adopt if not already adopted
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	projectName := filepath.Base(absDir)
	if _, ok := cfg.Projects[projectName]; !ok {
		fmt.Printf("Auto-adopting %s (%s.localhost)...\n", projectName, projectName)
		if _, err := adopt.Adopt(absDir, "", false); err != nil {
			return fmt.Errorf("auto-adopt failed: %w", err)
		}
	}

	// 2. Ensure gateway is running
	running, err := gateway.IsRunning(ctx)
	if err != nil {
		return fmt.Errorf("checking gateway: %w", err)
	}
	if !running {
		fmt.Println("Starting caddy-atc gateway...")
		if err := gateway.Up(ctx); err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
	}

	// 3. Detect compose files
	composeFiles, err := DetectComposeFiles(absDir)
	if err != nil {
		return err
	}

	// 4. Generate stripped files
	strippedFiles, err := GenerateStrippedFiles(composeFiles, opts.KeepPorts)
	if err != nil {
		return err
	}

	fmt.Printf("Generated %s (ports stripped)\n", filepath.Base(strippedFiles[0]))

	// 5. Build environment with COMPOSE_FILE pointing to stripped files
	composeFileEnv := BuildComposeFileEnv(strippedFiles)
	env := config.FilterEnv("COMPOSE_FILE")
	env = append(env, "COMPOSE_FILE="+composeFileEnv)

	// 6. Execute command
	if len(opts.Command) == 0 {
		return runDefault(ctx, absDir, env)
	}

	return execUserCommand(absDir, env, opts.Command)
}

// runDefault runs `docker compose up -d` and returns.
func runDefault(ctx context.Context, dir string, env []string) error {
	fmt.Println("Running: docker compose up -d")

	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d")
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Println("\nContainers started. The caddy-atc watcher will set up routes automatically.")
	fmt.Println("Tip: Add .caddy-atc-compose*.yml to your .gitignore")
	return nil
}

// execUserCommand replaces the current process with the user's command.
func execUserCommand(dir string, env []string, args []string) error {
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", args[0])
	}

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("changing to project directory: %w", err)
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))

	// syscall.Exec replaces the process — signals go directly to the new command
	return syscall.Exec(binary, args, env)
}

// Stop runs docker compose down using the stripped compose file.
func Stop(ctx context.Context, dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}

	strippedPath := filepath.Join(absDir, strippedPrefix+".yml")
	if _, err := os.Stat(strippedPath); os.IsNotExist(err) {
		fmt.Println("No stripped compose file found. Running: docker compose down")
		cmd := exec.CommandContext(ctx, "docker", "compose", "down")
		cmd.Dir = absDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	env := config.FilterEnv("COMPOSE_FILE")
	env = append(env, "COMPOSE_FILE="+strippedPath)

	overridePath := filepath.Join(absDir, strippedPrefix+".override.yml")
	if _, err := os.Stat(overridePath); err == nil {
		env[len(env)-1] = "COMPOSE_FILE=" + strippedPath + ":" + overridePath
	}

	fmt.Println("Running: docker compose down")
	cmd := exec.CommandContext(ctx, "docker", "compose", "down")
	cmd.Dir = absDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}

	os.Remove(strippedPath)
	os.Remove(overridePath)
	fmt.Println("Stripped compose files cleaned up.")

	return nil
}
