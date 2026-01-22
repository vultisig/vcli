package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/vultisig/vultisig-go/address"
	"github.com/vultisig/vultisig-go/common"
)

func newPolicyGenerateCmd() *cobra.Command {
	var pluginID, from, to, amount, frequency, vaultName, toVaultName, output string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a complete policy configuration file",
		Long: `Generate a complete policy JSON file with all addresses filled in.

Asset shortcuts:
  eth, btc, sol, rune, bnb, avax, matic  - Native tokens
  usdc, usdt, dai                        - Stablecoins (Ethereum)
  usdc:arbitrum                          - Specify chain explicitly

Frequency options:
  one-time, minutely, hourly, daily, weekly, bi-weekly, monthly

Vault selection:
  --vault       Source vault (default: first imported vault)
  --to-vault    Destination vault for sends (default: same as --vault)

Examples:
  # Swap ETH to USDC (same vault)
  vcli policy generate --from eth --to usdc --amount 0.01

  # Swap with explicit vault
  vcli policy generate --from eth --to usdc --amount 0.01 --vault FastPlugin1

  # Send ETH from Plugin1 to Plugin2
  vcli policy generate --from eth --to eth --amount 0.1 --vault Plugin1 --to-vault Plugin2

  # Output to file
  vcli policy generate --from eth --to usdc --amount 0.01 --output swap.json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyGenerate(pluginID, from, to, amount, frequency, vaultName, toVaultName, output)
		},
	}

	cmd.Flags().StringVar(&pluginID, "plugin", "dca", "Plugin ID or alias")
	cmd.Flags().StringVar(&from, "from", "", "Source asset (required)")
	cmd.Flags().StringVar(&to, "to", "", "Destination asset (required)")
	cmd.Flags().StringVar(&amount, "amount", "", "Amount in human units (required)")
	cmd.Flags().StringVar(&frequency, "frequency", "one-time", "Frequency: one-time, minutely, hourly, daily, weekly, bi-weekly, monthly")
	cmd.Flags().StringVar(&vaultName, "vault", "", "Source vault name (default: first vault)")
	cmd.Flags().StringVar(&toVaultName, "to-vault", "", "Destination vault name for sends (default: same as --vault)")
	cmd.Flags().StringVar(&output, "output", "", "Output file (default: stdout)")

	cmd.MarkFlagRequired("from")
	cmd.MarkFlagRequired("to")
	cmd.MarkFlagRequired("amount")

	return cmd
}

func runPolicyGenerate(pluginID, from, to, amount, frequency, vaultName, toVaultName, output string) error {
	// Load source vault
	fromVault, err := GetVaultByName(vaultName)
	if err != nil {
		return err
	}

	// Load destination vault (defaults to source vault)
	toVault := fromVault
	if toVaultName != "" {
		toVault, err = GetVaultByName(toVaultName)
		if err != nil {
			return err
		}
	}

	// Resolve assets
	fromAsset := ResolveAsset(from)
	toAsset := ResolveAsset(to)

	// Derive addresses
	fromAddr, err := deriveAddressForChain(fromVault, fromAsset.Chain)
	if err != nil {
		return fmt.Errorf("derive from address: %w", err)
	}

	toAddr, err := deriveAddressForChain(toVault, toAsset.Chain)
	if err != nil {
		return fmt.Errorf("derive to address: %w", err)
	}

	// Convert amount
	amountSmallest := ConvertToSmallestUnit(amount, fromAsset)

	// Build recipe
	recipe := map[string]any{
		"from": map[string]string{
			"chain":   fromAsset.Chain,
			"token":   fromAsset.Token,
			"address": fromAddr,
		},
		"to": map[string]string{
			"chain":   toAsset.Chain,
			"token":   toAsset.Token,
			"address": toAddr,
		},
		"fromAmount": amountSmallest,
		"frequency":  frequency,
	}

	// Validate recipe with plugin server
	err = validateRecipeWithPlugin(pluginID, recipe)
	if err != nil {
		return fmt.Errorf("recipe validation failed: %w", err)
	}

	// Fetch plugin pricing to build matching billing entries
	billing, err := fetchPluginBilling(pluginID)
	if err != nil {
		// If we can't fetch pricing, use empty billing (plugin may not have pricing)
		billing = []map[string]any{}
	}

	// Build policy with recipe and billing
	policy := map[string]any{
		"recipe":  recipe,
		"billing": billing,
	}

	jsonBytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	// Output
	if output != "" {
		err = os.WriteFile(output, jsonBytes, 0644)
		if err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		fmt.Printf("Policy written to %s\n", output)
	} else {
		fmt.Println(string(jsonBytes))
	}

	// Print summary
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Policy Summary:")
	fmt.Fprintf(os.Stderr, "  From: %s %s (%s)\n", amount, from, fromAsset.Chain)
	fmt.Fprintf(os.Stderr, "        %s [%s]\n", fromAddr, fromVault.Name)
	fmt.Fprintf(os.Stderr, "  To:   %s (%s)\n", to, toAsset.Chain)
	fmt.Fprintf(os.Stderr, "        %s [%s]\n", toAddr, toVault.Name)
	fmt.Fprintf(os.Stderr, "  Amount: %s (smallest unit)\n", amountSmallest)
	fmt.Fprintf(os.Stderr, "  Frequency: %s\n", frequency)

	// Print next step
	if output != "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "Next: vcli policy add --plugin %s --policy-file %s --password <password>\n", pluginID, output)
	}

	return nil
}

func deriveAddressForChain(vault *LocalVault, chainName string) (string, error) {
	chain, err := common.FromString(chainName)
	if err != nil {
		return "", fmt.Errorf("unknown chain: %s", chainName)
	}

	// Use EdDSA key for Solana, ECDSA for everything else
	pubKey := vault.PublicKeyECDSA
	if chain == common.Solana {
		pubKey = vault.PublicKeyEdDSA
	}

	addr, _, _, err := address.GetAddress(pubKey, vault.HexChainCode, chain)
	if err != nil {
		return "", fmt.Errorf("derive address for %s: %w", chainName, err)
	}

	return addr, nil
}

func validateRecipeWithPlugin(pluginID string, recipe map[string]any) error {
	pluginServerURL, err := GetPluginServerURL(pluginID)
	if err != nil {
		return fmt.Errorf("get plugin server URL: %w", err)
	}

	reqBody, err := json.Marshal(map[string]any{
		"configuration": recipe,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(pluginServerURL+"/plugin/recipe-specification/suggest", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s", extractErrorMessage(body))
	}

	return nil
}

func extractErrorMessage(body []byte) string {
	var resp map[string]any
	err := json.Unmarshal(body, &resp)
	if err != nil {
		return string(body)
	}
	if msg, ok := resp["message"].(string); ok {
		return msg
	}
	return string(body)
}

// fetchPluginBilling fetches the plugin's pricing and converts it to billing entries.
// The billing entries must match the plugin's pricing count for policy creation to succeed.
// Uses uint64 for amount to match verifier's expected type.
// Uses the public /plugins endpoint (no auth required) instead of /plugin/{id} (requires auth).
func fetchPluginBilling(pluginID string) ([]map[string]any, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	resolvedID := ResolvePluginID(pluginID)

	url := fmt.Sprintf("%s/plugins", cfg.Verifier)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch plugins: status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Plugins []struct {
				ID      string `json:"id"`
				Pricing []struct {
					Type      string  `json:"type"`
					Frequency *string `json:"frequency"`
					Amount    int64   `json:"amount"`
					Asset     string  `json:"asset"`
				} `json:"pricing"`
			} `json:"plugins"`
		} `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	var targetPricing []struct {
		Type      string  `json:"type"`
		Frequency *string `json:"frequency"`
		Amount    int64   `json:"amount"`
		Asset     string  `json:"asset"`
	}
	for _, p := range result.Data.Plugins {
		if p.ID == resolvedID {
			targetPricing = p.Pricing
			break
		}
	}

	var billing []map[string]any
	for _, p := range targetPricing {
		frequency := ""
		if p.Frequency != nil {
			frequency = *p.Frequency
		}
		entry := map[string]any{
			"type":      p.Type,
			"frequency": frequency,
			"amount":    uint64(p.Amount),
		}
		billing = append(billing, entry)
	}

	return billing, nil
}
