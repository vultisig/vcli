package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func NewVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verification and monitoring commands",
	}

	cmd.AddCommand(newVerifyTransactionsCmd())
	cmd.AddCommand(newVerifyPolicyCmd())
	cmd.AddCommand(newVerifyHealthCmd())

	return cmd
}

func newVerifyTransactionsCmd() *cobra.Command {
	var policyID string
	var pluginID string
	var limit int

	cmd := &cobra.Command{
		Use:   "transactions",
		Short: "Check transaction history for a policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if policyID != "" {
				return runVerifyPolicyTransactions(policyID, limit)
			}
			if pluginID != "" {
				return runVerifyPluginTransactions(pluginID, limit)
			}
			return fmt.Errorf("specify --policy or --plugin")
		},
	}

	cmd.Flags().StringVarP(&policyID, "policy", "p", "", "Policy ID to check")
	cmd.Flags().StringVarP(&pluginID, "plugin", "P", "", "Plugin ID to check all transactions")
	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "Number of transactions to show")

	return cmd
}

func newVerifyPolicyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "policy [policy-id]",
		Short: "Verify policy status and execution state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifyPolicy(args[0])
		},
	}
}

func newVerifyHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check health of all services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifyHealth()
		},
	}
}

func runVerifyPolicyTransactions(policyID string, limit int) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}

	fmt.Printf("Fetching transactions for policy %s...\n\n", policyID)

	url := fmt.Sprintf("%s/plugin/policies/%s/history?take=%d", cfg.Verifier, policyID, limit)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if data, ok := result["data"].([]interface{}); ok {
		if len(data) == 0 {
			fmt.Println("No transactions found for this policy.")
			fmt.Println("\nThe plugin may not have executed any transactions yet.")
			return nil
		}

		fmt.Printf("Found %d transactions:\n\n", len(data))
		for i, tx := range data {
			txMap := tx.(map[string]interface{})
			fmt.Printf("%d. Transaction:\n", i+1)
			fmt.Printf("   ID: %v\n", txMap["id"])
			fmt.Printf("   Status: %v\n", txMap["status"])
			fmt.Printf("   Chain: %v\n", txMap["chain"])
			fmt.Printf("   TxHash: %v\n", txMap["tx_hash"])
			fmt.Printf("   Created: %v\n", txMap["created_at"])
			fmt.Println()
		}
	} else {
		prettyJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(prettyJSON))
	}

	return nil
}

func runVerifyPluginTransactions(pluginID string, limit int) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}

	fmt.Printf("Fetching transactions for plugin %s...\n\n", pluginID)

	url := fmt.Sprintf("%s/plugin/transactions?plugin_id=%s&take=%d", cfg.Verifier, pluginID, limit)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	prettyJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(prettyJSON))

	return nil
}

func runVerifyPolicy(policyID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}

	fmt.Printf("Verifying policy %s...\n\n", policyID)

	url := fmt.Sprintf("%s/plugin/policy/%s", cfg.Verifier, policyID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("policy not found: %s", string(body))
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if data, ok := result["data"].(map[string]interface{}); ok {
		fmt.Println("Policy Status:")
		fmt.Printf("  ID: %v\n", data["id"])
		fmt.Printf("  Plugin: %v\n", data["plugin_id"])
		fmt.Printf("  Active: %v\n", data["active"])
		fmt.Printf("  Version: %v\n", data["policy_version"])
		fmt.Printf("  Public Key: %v\n", data["public_key"])
		fmt.Printf("  Created: %v\n", data["created_at"])

		if billing, ok := data["billing"].([]interface{}); ok && len(billing) > 0 {
			fmt.Println("\nBilling:")
			for _, b := range billing {
				bMap := b.(map[string]interface{})
				fmt.Printf("  - Type: %v, Amount: %v\n", bMap["type"], bMap["amount"])
			}
		}
	} else {
		prettyJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(prettyJSON))
	}

	return nil
}

func runVerifyHealth() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Println("Checking service health...")

	services := []struct {
		name string
		url  string
	}{
		{"Verifier", cfg.Verifier + "/healthz"},
		{"Fee Plugin", cfg.FeePlugin + "/healthz"},
		{"DCA Plugin", cfg.DCAPlugin + "/healthz"},
		{"Fast Vault Server", FastVaultServer + "/healthz"},
		{"Relay Server", RelayServer},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, svc := range services {
		req, _ := http.NewRequestWithContext(ctx, "GET", svc.url, nil)
		resp, err := http.DefaultClient.Do(req)

		status := "DOWN"
		if err == nil && resp.StatusCode == http.StatusOK {
			status = "OK"
		}
		if resp != nil {
			resp.Body.Close()
		}

		if status == "OK" {
			fmt.Printf("  ✓ %s: %s\n", svc.name, status)
		} else {
			fmt.Printf("  ✗ %s: %s\n", svc.name, status)
		}
	}

	fmt.Println("\nDocker Services:")
	fmt.Println("  Run 'docker compose ps' in devenv/ to check infrastructure")

	return nil
}
