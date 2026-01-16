package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func NewServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Manage local development services",
		Long: `Manage local development services running in Docker.

All services run as Docker containers. Use 'vcli start' for the recommended
way to start all services. This command provides more granular control.
`,
	}

	cmd.AddCommand(newServicesStartCmd())
	cmd.AddCommand(newServicesStopCmd())
	cmd.AddCommand(newServicesLogsCmd())
	cmd.AddCommand(newServicesInitCmd())
	cmd.AddCommand(newServicesStatusCmd())

	return cmd
}

func newServicesInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize local development environment",
		Long: `Initialize the local development environment.

This will:
1. Start Docker infrastructure (postgres, redis, minio)
2. Create required databases
3. Initialize MinIO buckets

For a full setup including verifier and plugins, use 'vcli start' instead.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesInit()
		},
	}
}

func newServicesStartCmd() *cobra.Command {
	var services []string
	var all bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start services",
		Long: `Start local development services (Docker containers).

Available services:
  - infra: Docker infrastructure (postgres, redis, minio)
  - verifier: Verifier API server
  - worker: Verifier worker
  - dca-server: DCA plugin server
  - dca-worker: DCA worker
  - dca-scheduler: DCA scheduler
  - dca-tx-indexer: DCA transaction indexer

Use --all to start all services, or use 'vcli start' for the recommended setup.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				services = []string{"postgres", "redis", "minio", "minio-init", "verifier", "worker", "dca-server", "dca-worker", "dca-scheduler", "dca-tx-indexer"}
			}
			return runServicesStart(services)
		},
	}

	cmd.Flags().StringSliceVarP(&services, "services", "s", []string{}, "Services to start")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Start all services")

	return cmd
}

func newServicesStopCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop services",
		Long: `Stop local development services (Docker containers).

Use --all to stop all services including infrastructure.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesStop(all)
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, "Stop all services including infrastructure")

	return cmd
}

func newServicesLogsCmd() *cobra.Command {
	var service string
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View service logs",
		Long: `View logs for Docker containers.

Examples:
  vcli services logs -s verifier
  vcli services logs -s verifier -f
  vcli services logs  # all services
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesLogs(service, follow)
		},
	}

	cmd.Flags().StringVarP(&service, "service", "s", "", "Service name (container name without vultisig- prefix)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}

func newServicesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesStatus()
		},
	}
}

func runServicesInit() error {
	fmt.Println("Initializing local development environment...")

	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yaml not found at %s", localDir)
	}

	fmt.Println("\n1. Starting Docker infrastructure...")
	// Start only infrastructure services
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", "postgres", "redis", "minio", "minio-init")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start docker: %w", err)
	}

	fmt.Println("\n2. Waiting for services to be healthy...")
	cmd = exec.Command("docker", "compose", "-f", composeFile, "ps")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	fmt.Println("\n3. Infrastructure ready!")
	fmt.Println("\nNext steps:")
	fmt.Println("  vcli start                  # Start all services (recommended)")
	fmt.Println("  vcli services start --all   # Or start services individually")

	return nil
}

func runServicesStart(services []string) error {
	if len(services) == 0 {
		fmt.Println("No services specified. Use -s to specify services or --all to start all.")
		fmt.Println("\nAvailable services: postgres, redis, minio, verifier, worker, dca-server, dca-worker, dca-scheduler, dca-tx-indexer")
		fmt.Println("\nOr use 'vcli start' to start everything (recommended).")
		return nil
	}

	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yaml not found at %s", localDir)
	}

	fmt.Printf("Starting services: %v\n", services)

	args := []string{"compose", "-f", composeFile, "up", "-d"}
	args = append(args, services...)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	fmt.Println("\nServices started. View logs with: vcli services logs -s <service>")
	return nil
}

func runServicesStop(all bool) error {
	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	if all {
		fmt.Println("Stopping all services...")
		cmd := exec.Command("docker", "compose", "-f", composeFile, "down")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	} else {
		// Stop only application services, keep infrastructure
		fmt.Println("Stopping application services (keeping infrastructure)...")
		services := []string{"verifier", "worker", "dca-server", "dca-worker", "dca-scheduler", "dca-tx-indexer"}
		args := []string{"compose", "-f", composeFile, "stop"}
		args = append(args, services...)
		cmd := exec.Command("docker", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		fmt.Println("Done. Use --all to also stop Docker infrastructure.")
	}

	return nil
}

func runServicesLogs(service string, follow bool) error {
	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	args := []string{"compose", "-f", composeFile, "logs"}
	if follow {
		args = append(args, "-f")
	}
	if service != "" {
		args = append(args, service)
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runServicesStatus() error {
	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	fmt.Println("Docker container status:")
	fmt.Println()

	cmd := exec.Command("docker", "compose", "-f", composeFile, "ps", "--format", "table {{.Name}}\t{{.Status}}\t{{.Ports}}")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
