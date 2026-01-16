package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorCyan   = "\033[0;36m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

func NewStartCmd() *cobra.Command {
	var skipDCA bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start all local development services",
		Long: `Start all local development services using Docker.

All services run as Docker containers - no local repo clones needed.

Services started:
1. PostgreSQL (database)
2. Redis (task queue)
3. MinIO (object storage)
4. Verifier API server
5. Verifier Worker
6. DCA Plugin Server
7. DCA Plugin Worker
8. DCA Scheduler
9. DCA TX Indexer

Prerequisites:
- Docker must be installed and running
- No local repo clones required

Logs:
- Use 'docker logs <container-name>' to view logs
- Container names: vultisig-verifier, vultisig-worker, vultisig-dca, etc.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(skipDCA)
		},
	}

	cmd.Flags().BoolVar(&skipDCA, "skip-dca", false, "Skip starting DCA plugin services")

	return cmd
}

func runStart(skipDCA bool) error {
	startTime := time.Now()

	fmt.Println("============================================")
	fmt.Println("  Vultisig Local Dev Environment Startup")
	fmt.Println("============================================")
	fmt.Println()

	// Find docker-compose file
	localDir := findLocalDir()
	composeFile := filepath.Join(localDir, "docker-compose.full.yaml")

	// Fall back to basic docker-compose.yaml if full doesn't exist
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		composeFile = filepath.Join(localDir, "docker-compose.yaml")
	}

	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yaml not found at %s\nRun from vcli/local/ directory or ensure docker-compose files exist", localDir)
	}

	fmt.Printf("Using: %s\n", composeFile)
	fmt.Println()

	// Step 1: Stop existing containers
	fmt.Printf("%s[1/4]%s Cleaning up existing containers...\n", colorYellow, colorReset)

	downCmd := exec.Command("docker", "compose", "-f", composeFile, "down", "-v", "--remove-orphans")
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	downCmd.Run() // Ignore errors - containers might not exist
	time.Sleep(2 * time.Second)
	fmt.Printf("%s✓%s Cleanup complete\n", colorGreen, colorReset)

	// Step 2: Start all containers
	fmt.Println()
	fmt.Printf("%s[2/4]%s Starting Docker containers...\n", colorYellow, colorReset)

	// Build services list based on skipDCA flag
	services := []string{"postgres", "redis", "minio", "minio-init", "verifier", "worker"}
	if !skipDCA {
		services = append(services, "dca-server", "dca-worker", "dca-scheduler", "dca-tx-indexer")
	}

	upArgs := []string{"compose", "-f", composeFile, "up", "-d"}
	upArgs = append(upArgs, services...)

	upCmd := exec.Command("docker", upArgs...)
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	err := upCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start docker containers: %w\nMake sure Docker is running and images are available", err)
	}
	fmt.Printf("%s✓%s Containers started\n", colorGreen, colorReset)

	// Step 3: Wait for services to be healthy
	fmt.Println()
	fmt.Printf("%s[3/4]%s Waiting for services to be ready...\n", colorYellow, colorReset)

	// Wait for PostgreSQL
	fmt.Print("  PostgreSQL...")
	if waitForDocker("vultisig-postgres", 30*time.Second) {
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf(" %s✗%s (check: docker logs vultisig-postgres)\n", colorRed, colorReset)
	}

	// Wait for Redis
	fmt.Print("  Redis...")
	if waitForRedis(30 * time.Second) {
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf(" %s✗%s (check: docker logs vultisig-redis)\n", colorRed, colorReset)
	}

	// Wait for MinIO
	fmt.Print("  MinIO...")
	time.Sleep(3 * time.Second) // MinIO init needs time
	fmt.Printf(" %s✓%s\n", colorGreen, colorReset)

	// Wait for Verifier API
	fmt.Print("  Verifier API...")
	if waitForHealthy("http://localhost:8080/plugins", 120*time.Second) {
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf(" %s✗%s (check: docker logs vultisig-verifier)\n", colorRed, colorReset)
	}

	// Wait for Verifier Worker (no health endpoint, just wait a bit)
	fmt.Print("  Verifier Worker...")
	time.Sleep(5 * time.Second)
	fmt.Printf(" %s✓%s\n", colorGreen, colorReset)

	if !skipDCA {
		// Wait for DCA Plugin API
		fmt.Print("  DCA Plugin API...")
		if waitForHealthy("http://localhost:8082/spec", 120*time.Second) {
			fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
		} else {
			fmt.Printf(" %s✗%s (check: docker logs vultisig-dca)\n", colorRed, colorReset)
		}

		// Wait for DCA Worker (no health endpoint)
		fmt.Print("  DCA Plugin Worker...")
		time.Sleep(3 * time.Second)
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)

		// Wait for DCA Scheduler
		fmt.Print("  DCA Scheduler...")
		time.Sleep(2 * time.Second)
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)

		// Wait for DCA TX Indexer
		fmt.Print("  DCA TX Indexer...")
		time.Sleep(2 * time.Second)
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
	}

	// Step 4: Seed plugins
	fmt.Println()
	fmt.Printf("%s[4/4]%s Seeding plugins...\n", colorYellow, colorReset)

	seedFile := filepath.Join(localDir, "seed-plugins.sql")
	if _, err := os.Stat(seedFile); err == nil {
		seedData, _ := os.ReadFile(seedFile)
		seedCmd := exec.Command("docker", "exec", "-i", "vultisig-postgres", "psql", "-U", "vultisig", "-d", "vultisig-verifier")
		seedCmd.Stdin = strings.NewReader(string(seedData))
		seedCmd.Run()
		fmt.Printf("%s✓%s Plugins seeded\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s!%s No seed file found (optional)\n", colorYellow, colorReset)
	}

	// Print summary
	elapsed := time.Since(startTime)
	printDockerStartupSummary(elapsed, skipDCA)

	return nil
}

func waitForDocker(containerName string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
			cmd := exec.Command("docker", "inspect", "--format={{.State.Health.Status}}", containerName)
			out, err := cmd.Output()
			if err == nil && strings.TrimSpace(string(out)) == "healthy" {
				return true
			}
			// Also check if container is running (for containers without health checks)
			cmd = exec.Command("docker", "inspect", "--format={{.State.Running}}", containerName)
			out, err = cmd.Output()
			if err == nil && strings.TrimSpace(string(out)) == "true" {
				// Container is running, check if it has a health status
				cmd = exec.Command("docker", "inspect", "--format={{.State.Health}}", containerName)
				out, _ = cmd.Output()
				if strings.TrimSpace(string(out)) == "<nil>" {
					// No health check defined, assume healthy if running
					return true
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func waitForRedis(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
			cmd := exec.Command("docker", "exec", "vultisig-redis", "redis-cli", "-a", "vultisig", "ping")
			out, err := cmd.Output()
			if err == nil && strings.TrimSpace(string(out)) == "PONG" {
				return true
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func waitForHealthy(url string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
			resp, err := http.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return true
			}
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func findLocalDir() string {
	paths := []string{
		".",
		"local",
	}

	for _, p := range paths {
		dockerCompose := filepath.Join(p, "docker-compose.yaml")
		dockerComposeFull := filepath.Join(p, "docker-compose.full.yaml")
		if _, err := os.Stat(dockerCompose); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
		if _, err := os.Stat(dockerComposeFull); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return "local"
}

func printDockerStartupSummary(elapsed time.Duration, skipDCA bool) {
	fmt.Println()
	fmt.Printf("%s┌─────────────────────────────────────────────────────────────────┐%s\n", colorCyan, colorReset)
	fmt.Printf("%s│%s %sSTARTUP COMPLETE%s                                                %s│%s\n", colorCyan, colorReset, colorBold, colorReset, colorCyan, colorReset)
	fmt.Printf("%s├─────────────────────────────────────────────────────────────────┤%s\n", colorCyan, colorReset)
	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  Services (Docker containers):                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    Verifier API         localhost:8080                         %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    Verifier Worker      (background)                           %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)

	if !skipDCA {
		fmt.Printf("%s│%s    DCA Plugin API       localhost:8082                         %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
		fmt.Printf("%s│%s    DCA Plugin Worker    (background)                           %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
		fmt.Printf("%s│%s    DCA Scheduler        (background)                           %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
		fmt.Printf("%s│%s    DCA TX Indexer       (background)                           %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	}

	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  Infrastructure:                                               %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    PostgreSQL           localhost:5432                         %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    Redis                localhost:6379                         %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    MinIO                localhost:9000 (console: 9090)         %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  External Services:                                            %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    Relay:       https://api.vultisig.com/router                %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s    Vultiserver: https://api.vultisig.com                       %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  View logs:  docker logs <container-name>                      %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  Containers: vultisig-verifier, vultisig-worker, vultisig-dca  %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s  Total startup time: %s%ds%s                                        %s│%s\n", colorCyan, colorReset, colorBold, int(elapsed.Seconds()), colorReset, colorCyan, colorReset)
	fmt.Printf("%s│%s                                                                 %s│%s\n", colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("%s└─────────────────────────────────────────────────────────────────┘%s\n", colorCyan, colorReset)

	fmt.Println()
	fmt.Printf("%sReady for vault import!%s\n", colorGreen, colorReset)
	fmt.Println()
	fmt.Println("Next: vcli vault import --password <password>")
}
