package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/g-brodiei/caddy-atc/internal/adopt"
	"github.com/g-brodiei/caddy-atc/internal/config"
	"github.com/g-brodiei/caddy-atc/internal/gateway"
	"github.com/g-brodiei/caddy-atc/internal/routes"
	"github.com/g-brodiei/caddy-atc/internal/watcher"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "caddy-atc",
		Short: "Local development gateway - route projects by hostname",
		Long:  "caddy-atc eliminates Docker port conflicts by routing HTTP traffic through a single Caddy gateway using hostname-based routing (project.localhost).",
	}

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(downCmd())
	rootCmd.AddCommand(adoptCmd())
	rootCmd.AddCommand(unadoptCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(routesCmd())
	rootCmd.AddCommand(trustCmd())
	rootCmd.AddCommand(logsCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func upCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start the caddy-atc gateway and watcher",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Start gateway
			fmt.Println("Starting caddy-atc gateway...")
			if err := gateway.Up(ctx); err != nil {
				return err
			}

			// Start watcher in foreground
			fmt.Println("Starting watcher (press Ctrl+C to stop)...")
			return runWatcher(ctx)
		},
	}
}

func downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop the caddy-atc gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Stop watcher if running (via PID file)
			stopWatcher()

			// Stop gateway
			return gateway.Down(ctx)
		},
	}
}

func adoptCmd() *cobra.Command {
	var hostname string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "adopt [directory]",
		Short: "Register a project for automatic routing",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}

			fmt.Println("Scanning docker-compose.yml...")
			fmt.Println()

			result, err := adopt.Adopt(dir, hostname, dryRun)
			if err != nil {
				return err
			}

			// Display results using the exported FindPrimaryService
			primaryIdx := adopt.FindPrimaryService(result.HTTPServices)
			fmt.Println("Detected HTTP services:")
			for i, svc := range result.HTTPServices {
				h := svc.Name + "." + result.Hostname
				if i == primaryIdx {
					h = result.Hostname
				}
				fmt.Printf("  %-12s (port %-5s) -> %s\n", svc.Name, svc.Port, h)
			}

			if len(result.SkippedServices) > 0 {
				fmt.Println()
				fmt.Println("Skipped (non-HTTP):")
				for _, svc := range result.SkippedServices {
					ports := strings.Join(svc.Ports, ", ")
					if ports == "" {
						ports = "no ports"
					}
					fmt.Printf("  %-12s (%s)\n", svc.Name, ports)
				}
			}

			fmt.Println()
			if dryRun {
				fmt.Println("(dry run - no changes saved)")
			} else {
				fmt.Printf("Saved to %s\n", config.ProjectsPath())
			}

			// Check if any HTTP service uses hostname-based site address
			fmt.Println()
			fmt.Printf("NOTE: If your project's Caddyfile uses '%s' as the site address,\n", result.Hostname)
			fmt.Println("      change it to ':80' so it accepts HTTP from the gateway.")
			fmt.Println()
			fmt.Println("Start your project normally - caddy-atc will auto-connect it.")

			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "", "Override base hostname (default: <dirname>.localhost)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without saving")

	return cmd
}

func unadoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unadopt [directory]",
		Short: "Remove a project from automatic routing",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}

			if err := adopt.Unadopt(dir); err != nil {
				return err
			}

			fmt.Println("Project removed from caddy-atc.")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show gateway health and active routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Check gateway
			running, err := gateway.IsRunning(ctx)
			if err != nil {
				return err
			}

			if running {
				fmt.Println("Gateway: running")
			} else {
				fmt.Println("Gateway: stopped")
				return nil
			}

			// Check watcher
			if isWatcherRunning() {
				fmt.Println("Watcher: running")
			} else {
				fmt.Println("Watcher: stopped")
			}

			fmt.Println()

			// List routes
			activeRoutes, err := routes.ListActive(ctx)
			if err != nil {
				return fmt.Errorf("listing routes: %w", err)
			}

			if len(activeRoutes) == 0 {
				fmt.Println("No active routes.")
				return nil
			}

			fmt.Printf("Active routes (%d):\n", len(activeRoutes))
			printRouteTable(activeRoutes)

			return nil
		},
	}
}

func routesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "routes",
		Short: "List all active routes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			activeRoutes, err := routes.ListActive(ctx)
			if err != nil {
				return err
			}

			if len(activeRoutes) == 0 {
				fmt.Println("No active routes.")
				return nil
			}

			printRouteTable(activeRoutes)
			return nil
		},
	}
}

func trustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trust",
		Short: "Install Caddy's root CA in system trust store",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gateway.Trust(cmd.Context())
		},
	}
}

func logsCmd() *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show watcher logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			logPath := config.LogPath()
			if _, err := os.Stat(logPath); os.IsNotExist(err) {
				fmt.Println("No watcher logs found.")
				return nil
			}

			if follow {
				return gateway.Logs(cmd.Context(), true)
			}

			data, err := os.ReadFile(logPath)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

func printRouteTable(activeRoutes []routes.ActiveRoute) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HOSTNAME\tCONTAINER\tPORT\tPROJECT\tSERVICE\tSTATUS")
	for _, r := range activeRoutes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Hostname, r.ContainerName, r.Port, r.Project, r.Service, r.Status)
	}
	w.Flush()
}

func runWatcher(ctx context.Context) error {
	if err := config.EnsureHomeDir(); err != nil {
		return err
	}

	// Set up logging
	logFile, err := os.OpenFile(config.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()

	logger := log.New(io.MultiWriter(os.Stdout, logFile), "[caddy-atc] ", log.LstdFlags)

	// Write PID file
	if err := os.WriteFile(config.PidPath(), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		logger.Printf("Warning: could not write PID file: %v", err)
	}
	defer os.Remove(config.PidPath())

	// Create watcher
	w, err := watcher.New(logger)
	if err != nil {
		return err
	}
	defer w.Close()

	// Handle signals
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return w.Run(ctx)
}

func stopWatcher() {
	data, err := os.ReadFile(config.PidPath())
	if err != nil {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(config.PidPath())
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(config.PidPath())
		return
	}

	// Verify the process is actually running before sending signal
	if proc.Signal(syscall.Signal(0)) != nil {
		// Process doesn't exist - stale PID file
		os.Remove(config.PidPath())
		return
	}

	// Verify it's a caddy-atc process by checking /proc/<pid>/cmdline
	if !isCaddyATCProcess(pid) {
		fmt.Printf("Warning: PID %d is not a caddy-atc process, removing stale PID file\n", pid)
		os.Remove(config.PidPath())
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Warning: could not stop watcher (PID %d): %v\n", pid, err)
	} else {
		fmt.Println("Watcher stopped.")
	}
	os.Remove(config.PidPath())
}

func isWatcherRunning() bool {
	data, err := os.ReadFile(config.PidPath())
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, sending signal 0 checks if process exists
	if proc.Signal(syscall.Signal(0)) != nil {
		return false
	}

	return isCaddyATCProcess(pid)
}

// isCaddyATCProcess checks /proc/<pid>/cmdline to verify it's a caddy-atc process.
func isCaddyATCProcess(pid int) bool {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		// Can't read cmdline - could be permission issue; assume it's ours
		// if the PID file exists and process is alive
		return true
	}
	return strings.Contains(string(cmdline), "caddy-atc")
}
