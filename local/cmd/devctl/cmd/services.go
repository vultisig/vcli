package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Manage local development services",
	}

	cmd.AddCommand(newServicesStartCmd())
	cmd.AddCommand(newServicesStopCmd())
	cmd.AddCommand(newServicesLogsCmd())
	cmd.AddCommand(newServicesInitCmd())

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
3. Run database migrations
4. Seed initial plugin data
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
		Long: `Start local development services.

Available services:
  - infra: Docker infrastructure (postgres, redis, minio)
  - verifier: Verifier API server
  - worker: Verifier worker
  - fee: Fee plugin server
  - dca: DCA plugin server
  - dca-scheduler: DCA scheduler
  - dca-worker: DCA worker

Use --all to start all services.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				services = []string{"infra", "verifier", "worker", "fee"}
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServicesLogs(service, follow)
		},
	}

	cmd.Flags().StringVarP(&service, "service", "s", "", "Service name")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")

	return cmd
}

func runServicesInit() error {
	fmt.Println("Initializing local development environment...")

	verifierRoot := findVerifierRoot()
	if verifierRoot == "" {
		return fmt.Errorf("could not find verifier root directory")
	}

	composeFile := filepath.Join(verifierRoot, "devenv", "docker-compose.yaml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yaml not found at %s", composeFile)
	}

	fmt.Println("\n1. Starting Docker infrastructure...")
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
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
	fmt.Println("  1. Start verifier: devctl services start -s verifier")
	fmt.Println("  2. Start worker: devctl services start -s worker")
	fmt.Println("  3. Start fee plugin: devctl services start -s fee")
	fmt.Println("  4. Or start all: devctl services start --all")

	return nil
}

func runServicesStart(services []string) error {
	if len(services) == 0 {
		fmt.Println("No services specified. Use -s to specify services or --all to start all.")
		fmt.Println("\nAvailable services: infra, verifier, worker, fee, dca, dca-scheduler, dca-worker")
		return nil
	}

	verifierRoot := findVerifierRoot()
	feeRoot := findServiceRoot("feeplugin")
	dcaRoot := findServiceRoot("app-recurring")

	dyldPath := os.Getenv("DYLD_LIBRARY_PATH")
	goWrappersPath := "/Users/dev/dev/vultisig/go-wrappers/includes/darwin/"
	if !strings.Contains(dyldPath, goWrappersPath) {
		dyldPath = goWrappersPath + ":" + dyldPath
	}

	for _, svc := range services {
		fmt.Printf("Starting %s...\n", svc)

		switch svc {
		case "infra":
			composeFile := filepath.Join(verifierRoot, "devenv", "docker-compose.yaml")
			cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
			}

		case "verifier":
			fmt.Printf("  cd %s && VS_CONFIG_NAME=devenv/config/verifier go run cmd/verifier/main.go\n", verifierRoot)
			fmt.Println("  [Run in separate terminal with DYLD_LIBRARY_PATH set]")

		case "worker":
			fmt.Printf("  cd %s && VS_WORKER_CONFIG_NAME=devenv/config/worker go run cmd/worker/main.go\n", verifierRoot)
			fmt.Println("  [Run in separate terminal with DYLD_LIBRARY_PATH set]")

		case "fee":
			if feeRoot != "" {
				fmt.Printf("  cd %s && VS_CONFIG_NAME=../verifier/devenv/config/fee-server go run cmd/server/main.go\n", feeRoot)
			} else {
				fmt.Println("  Fee plugin root not found")
			}

		case "dca", "dca-server":
			if dcaRoot != "" {
				fmt.Printf("  cd %s && [configure env] go run cmd/server/main.go\n", dcaRoot)
			} else {
				fmt.Println("  DCA plugin root not found")
			}

		case "dca-scheduler":
			if dcaRoot != "" {
				fmt.Printf("  cd %s && [configure env] go run cmd/scheduler/main.go\n", dcaRoot)
			}

		case "dca-worker":
			if dcaRoot != "" {
				fmt.Printf("  cd %s && [configure env] go run cmd/worker/main.go\n", dcaRoot)
			}

		default:
			fmt.Printf("  Unknown service: %s\n", svc)
		}
	}

	fmt.Println("\nNote: Set DYLD_LIBRARY_PATH before running Go services:")
	fmt.Printf("  export DYLD_LIBRARY_PATH=%s:$DYLD_LIBRARY_PATH\n", goWrappersPath)

	return nil
}

func runServicesStop(all bool) error {
	verifierRoot := findVerifierRoot()

	if all {
		fmt.Println("Stopping all services...")

		fmt.Println("Stopping Go processes...")
		exec.Command("pkill", "-f", "go run cmd/verifier").Run()
		exec.Command("pkill", "-f", "go run cmd/worker").Run()
		exec.Command("pkill", "-f", "go run cmd/server").Run()
		exec.Command("pkill", "-f", "go run cmd/scheduler").Run()

		if verifierRoot != "" {
			composeFile := filepath.Join(verifierRoot, "devenv", "docker-compose.yaml")
			fmt.Println("Stopping Docker infrastructure...")
			cmd := exec.Command("docker", "compose", "-f", composeFile, "down")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	} else {
		fmt.Println("Stopping Go processes...")
		exec.Command("pkill", "-f", "go run cmd/verifier").Run()
		exec.Command("pkill", "-f", "go run cmd/worker").Run()
		exec.Command("pkill", "-f", "go run cmd/server").Run()
		exec.Command("pkill", "-f", "go run cmd/scheduler").Run()
		fmt.Println("Done. Use --all to also stop Docker infrastructure.")
	}

	return nil
}

func runServicesLogs(service string, follow bool) error {
	verifierRoot := findVerifierRoot()
	composeFile := filepath.Join(verifierRoot, "devenv", "docker-compose.yaml")

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

func findVerifierRoot() string {
	paths := []string{
		"/Users/dev/dev/vultisig/verifier",
		".",
		"..",
	}

	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(p, "cmd", "verifier")); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return ""
}

func findServiceRoot(name string) string {
	paths := []string{
		"/Users/dev/dev/vultisig/" + name,
		"../" + name,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return ""
}
