package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check status of all services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Println("Service Status")
	fmt.Println("==============")

	services := []struct {
		name string
		url  string
	}{
		{"Verifier", cfg.Verifier + "/healthz"},
		{"Fee Plugin", cfg.FeePlugin + "/healthz"},
		{"DCA Plugin", cfg.DCAPlugin + "/healthz"},
	}

	for _, svc := range services {
		status := checkHealth(svc.url)
		if status {
			fmt.Printf("  %-15s %s\n", svc.name+":", "OK")
		} else {
			fmt.Printf("  %-15s %s\n", svc.name+":", "DOWN")
		}
	}

	fmt.Println("\nInfrastructure:")

	infraServices := []struct {
		name string
		url  string
	}{
		{"PostgreSQL", "localhost:5432"},
		{"Redis", "localhost:6379"},
		{"MinIO", "http://localhost:9000/minio/health/live"},
	}

	for _, svc := range infraServices {
		var status bool
		if svc.name == "MinIO" {
			status = checkHealth(svc.url)
		} else {
			status = checkPort(svc.url)
		}
		if status {
			fmt.Printf("  %-15s %s\n", svc.name+":", "OK")
		} else {
			fmt.Printf("  %-15s %s\n", svc.name+":", "DOWN")
		}
	}

	fmt.Println("\nVault:")
	if cfg.PublicKeyECDSA != "" {
		fmt.Printf("  Name: %s\n", cfg.VaultName)
		fmt.Printf("  Public Key (ECDSA): %s...\n", cfg.PublicKeyECDSA[:32])
	} else {
		fmt.Println("  No vault configured")
	}

	fmt.Println("\nConfig file:", ConfigPath())

	return nil
}

func checkHealth(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func checkPort(addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	d := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr, nil)
	_, err := d.Do(req)

	return err == nil || err.Error() != "connection refused"
}
