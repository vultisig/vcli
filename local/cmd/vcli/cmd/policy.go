package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	rtypes "github.com/vultisig/recipes/types"
	"github.com/vultisig/vultisig-go/address"
	"github.com/vultisig/vultisig-go/common"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func NewPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Policy management commands",
	}

	cmd.AddCommand(newPolicyListCmd())
	cmd.AddCommand(newPolicyAddCmd())
	cmd.AddCommand(newPolicyDeleteCmd())
	cmd.AddCommand(newPolicyInfoCmd())
	cmd.AddCommand(newPolicyHistoryCmd())
	cmd.AddCommand(newPolicyStatusCmd())
	cmd.AddCommand(newPolicyTransactionsCmd())
	cmd.AddCommand(newPolicyTriggerCmd())
	cmd.AddCommand(newPolicyGenerateCmd())

	return cmd
}

func newPolicyListCmd() *cobra.Command {
	var pluginID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies for a plugin",
		Long: `List all policies for a plugin.

Plugin ID can be an alias (dca, fee, sends) or full ID.
Run 'vcli plugin aliases' to see available aliases.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyList(ResolvePluginID(pluginID))
		},
	}

	cmd.Flags().StringVar(&pluginID, "plugin", "", "Plugin ID or alias (required)")
	cmd.MarkFlagRequired("plugin")

	return cmd
}

func newPolicyAddCmd() *cobra.Command {
	var pluginID string
	var configFile string
	var password string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new policy",
		Long: `Add a new policy for a plugin.

Plugin ID can be an alias (dca, fee, sends) or full ID.
Run 'vcli plugin aliases' to see available aliases.

The policy configuration should be a JSON file containing:
{
  "recipe": {
    // Recipe-specific configuration (varies by plugin)
  },
  "billing": [
    { "type": "once", "amount": 0 }
  ]
}

Example for DCA plugin (swap ETH to USDC):
{
  "recipe": {
    "from": { "chain": "Ethereum", "token": "", "address": "" },
    "to": { "chain": "Ethereum", "token": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", "address": "" },
    "fromAmount": "1000000000000000",
    "frequency": "daily"
  },
  "billing": [{ "type": "once", "amount": 0 }]
}

Environment variables:
  VAULT_PASSWORD  - Fast Vault password (or use --password flag)

Note: Requires authentication. Run 'vcli vault import' first.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			actualPassword := password
			if envPass := os.Getenv("VAULT_PASSWORD"); envPass != "" {
				actualPassword = envPass
			}
			if actualPassword == "" {
				var err error
				actualPassword, err = promptPassword("", "Enter Fast Vault password: ")
				if err != nil {
					return err
				}
			}
			return runPolicyAdd(ResolvePluginID(pluginID), configFile, actualPassword)
		},
	}

	cmd.Flags().StringVar(&pluginID, "plugin", "", "Plugin ID or alias (required)")
	cmd.Flags().StringVar(&configFile, "policy-file", "", "Policy configuration JSON file (required)")
	cmd.Flags().StringVar(&password, "password", "", "Fast Vault password (or set VAULT_PASSWORD env var)")
	cmd.MarkFlagRequired("plugin")
	cmd.MarkFlagRequired("policy-file")

	return cmd
}

func newPolicyDeleteCmd() *cobra.Command {
	var password string

	cmd := &cobra.Command{
		Use:   "delete [policy-id]",
		Short: "Delete a policy",
		Long: `Delete a policy by ID.

Environment variables:
  VAULT_PASSWORD  - Fast Vault password (or use --password flag)
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actualPassword := password
			if envPass := os.Getenv("VAULT_PASSWORD"); envPass != "" {
				actualPassword = envPass
			}
			if actualPassword == "" {
				return fmt.Errorf("password required: use --password or set VAULT_PASSWORD")
			}
			return runPolicyDelete(args[0], actualPassword)
		},
	}

	cmd.Flags().StringVar(&password, "password", "", "Vault password for TSS signing (or set VAULT_PASSWORD)")

	return cmd
}

func newPolicyInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [policy-id]",
		Short: "Show policy details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyInfo(args[0])
		},
	}
}

func newPolicyHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history [policy-id]",
		Short: "Show policy transaction history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyHistory(args[0])
		},
	}
}

func runPolicyList(pluginID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w\n\nRun 'vcli vault import' first", err)
	}

	vaults, err := ListVaults()
	if err != nil || len(vaults) == 0 {
		return fmt.Errorf("no vaults found. Import a vault first: vcli vault import")
	}
	publicKey := vaults[0].PublicKeyECDSA

	fmt.Printf("Fetching policies for plugin %s...\n", pluginID)
	fmt.Printf("  Vault: %s...\n\n", publicKey[:20])

	url := fmt.Sprintf("%s/plugin/policies/%s?public_key=%s", cfg.Verifier, pluginID, publicKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed (%d): %s", resp.StatusCode, string(body))
	}

	var policies []map[string]interface{}
	err = json.Unmarshal(body, &policies)
	if err != nil {
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		prettyJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(prettyJSON))
		return nil
	}

	if len(policies) == 0 {
		fmt.Println("No policies found for this plugin.")
		return nil
	}

	fmt.Printf("Found %d policies:\n\n", len(policies))
	for i, p := range policies {
		policyID := p["id"]
		active := p["active"]
		createdAt := p["created_at"]
		fmt.Printf("  %d. Policy ID: %v\n", i+1, policyID)
		fmt.Printf("     Active: %v\n", active)
		fmt.Printf("     Created: %v\n\n", createdAt)
	}

	return nil
}

func runPolicyAdd(pluginID, configFile string, password string) error {
	startTime := time.Now()

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w\n\nRun 'vcli vault import --password xxx' to authenticate first", err)
	}

	vaults, err := ListVaults()
	if err != nil || len(vaults) == 0 {
		return fmt.Errorf("no vaults found. Import a vault first: vcli vault import")
	}
	vault := vaults[0]

	configData, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var policyConfig map[string]interface{}
	err = json.Unmarshal(configData, &policyConfig)
	if err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}

	recipeConfig, ok := policyConfig["recipe"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid 'recipe' in config file")
	}

	// Auto-fill addresses from vault if empty
	recipeConfig, err = fillAddressesFromVault(recipeConfig, vault)
	if err != nil {
		return fmt.Errorf("fill addresses from vault: %w", err)
	}

	fmt.Printf("Creating policy for plugin %s...\n", pluginID)
	fmt.Printf("  Vault: %s (%s...)\n", vault.Name, vault.PublicKeyECDSA[:16])
	fmt.Printf("  Config: %s\n", configFile)

	// Step 1: Get plugin server URL
	pluginServerURL, err := getPluginServerURL(cfg.Verifier, pluginID)
	if err != nil {
		return fmt.Errorf("get plugin server URL: %w", err)
	}
	fmt.Printf("  Plugin Server: %s\n", pluginServerURL)

	// Step 2: Call plugin's suggest endpoint to get rules
	fmt.Println("\nFetching policy template from plugin...")
	policySuggest, err := getPluginPolicySuggest(pluginServerURL, recipeConfig)
	if err != nil {
		return fmt.Errorf("get policy suggest: %w", err)
	}
	fmt.Printf("  Rules: %d\n", len(policySuggest.GetRules()))
	if policySuggest.RateLimitWindow != nil {
		fmt.Printf("  Rate Limit Window: %ds\n", policySuggest.GetRateLimitWindow())
	}

	// Step 3: Build protobuf Policy
	policy, err := buildProtobufPolicy(pluginID, recipeConfig, policyConfig["billing"], policySuggest)
	if err != nil {
		return fmt.Errorf("build protobuf policy: %w", err)
	}

	// Step 4: Serialize to protobuf bytes, then base64
	policyBytes, err := proto.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal protobuf policy: %w", err)
	}
	recipeBase64 := base64.StdEncoding.EncodeToString(policyBytes)

	policyVersion := 1
	pluginVersion := "1.0.0"

	// Step 5: Create signature message and sign
	// Message format: {recipe}*#*{public_key}*#*{policy_version}*#*{plugin_version}
	signatureMessage := fmt.Sprintf("%s*#*%s*#*%d*#*%s",
		recipeBase64,
		vault.PublicKeyECDSA,
		policyVersion,
		pluginVersion,
	)

	// DEBUG: print message details
	fmt.Printf("\n  DEBUG: Signing message:\n")
	fmt.Printf("    Recipe (first 50 chars): %s...\n", recipeBase64[:min(50, len(recipeBase64))])
	fmt.Printf("    Public Key: %s\n", vault.PublicKeyECDSA)
	fmt.Printf("    Policy Version: %d\n", policyVersion)
	fmt.Printf("    Plugin Version: %s\n", pluginVersion)
	fmt.Printf("    Full message length: %d\n", len(signatureMessage))

	ethPrefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(signatureMessage), signatureMessage)
	messageHash := crypto.Keccak256([]byte(ethPrefixedMessage))
	hexMessage := hex.EncodeToString(messageHash)
	fmt.Printf("    Message hash: %s\n", hexMessage)

	fmt.Println("\nSigning policy with TSS keysign (2-of-2 with Fast Vault Server)...")

	if password == "" {
		return fmt.Errorf("password is required for TSS keysign. Use --password flag")
	}

	tss := NewTSSService(vault.LocalPartyID)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	derivePath := "m/44'/60'/0'/0/0"
	results, err := tss.KeysignWithFastVault(ctx, vault, []string{hexMessage}, derivePath, password)
	if err != nil {
		return fmt.Errorf("TSS keysign failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no signature result")
	}

	// Build signature in Ethereum format (R + S + V) - same as auth signing
	signature := "0x" + results[0].R + results[0].S + results[0].RecoveryID
	fmt.Printf("  DEBUG: Signature: %s\n", signature)
	fmt.Printf("  DEBUG: R: %s, S: %s, V: %s\n", results[0].R, results[0].S, results[0].RecoveryID)

	// Step 6: Build billing array for API request
	billingArray, err := buildBillingArray(policyConfig["billing"])
	if err != nil {
		return fmt.Errorf("build billing array: %w", err)
	}

	policyRequest := map[string]interface{}{
		"plugin_id":      pluginID,
		"public_key":     vault.PublicKeyECDSA,
		"plugin_version": pluginVersion,
		"policy_version": policyVersion,
		"signature":      signature,
		"recipe":         recipeBase64,
		"billing":        billingArray,
		"active":         true,
	}

	policyJSON, err := json.Marshal(policyRequest)
	if err != nil {
		return fmt.Errorf("marshal policy request: %w", err)
	}

	// Step 7: Submit to verifier
	fmt.Println("\nSubmitting policy to verifier...")

	url := cfg.Verifier + "/plugin/policy"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(policyJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit policy: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create policy failed (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	totalDuration := time.Since(startTime)

	// Extract recipe summary
	var fromChain, fromToken, fromAmount, toChain, toToken, frequency string
	if from, ok := recipeConfig["from"].(map[string]interface{}); ok {
		if c, ok := from["chain"].(string); ok {
			fromChain = c
		}
		if t, ok := from["token"].(string); ok && t != "" {
			fromToken = t
		} else {
			fromToken = "native"
		}
	}
	if to, ok := recipeConfig["to"].(map[string]interface{}); ok {
		if c, ok := to["chain"].(string); ok {
			toChain = c
		}
		if t, ok := to["token"].(string); ok && t != "" {
			toToken = t
		} else {
			toToken = "native"
		}
	}
	if amt, ok := recipeConfig["fromAmount"].(string); ok {
		fromAmount = amt
	}
	if freq, ok := recipeConfig["frequency"].(string); ok {
		frequency = freq
	} else {
		frequency = "one-time"
	}

	// Print completion report
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ POLICY ADDED SUCCESSFULLY                                       │")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Plugin:      %-50s │\n", pluginID)
	fmt.Printf("│  Vault:       %-50s │\n", vault.PublicKeyECDSA[:16]+"...")
	var policyID string
	if data, ok := result["data"].(map[string]interface{}); ok {
		if id, ok := data["id"].(string); ok {
			policyID = id
			fmt.Printf("│  Policy ID:   %-50s │\n", id)
		}
	}
	fmt.Println("│                                                                 │")
	fmt.Println("│  Summary:                                                      │")
	if fromChain != "" {
		fmt.Printf("│    From:      %-47s │\n", fmt.Sprintf("%s (%s)", fromToken, fromChain))
	}
	if toChain != "" {
		fmt.Printf("│    To:        %-47s │\n", fmt.Sprintf("%s (%s)", toToken, toChain))
	}
	if fromAmount != "" {
		fmt.Printf("│    Amount:    %-47s │\n", fromAmount)
	}
	fmt.Printf("│    Frequency: %-47s │\n", frequency)
	fmt.Printf("│    Rules:     %-47d │\n", len(policySuggest.GetRules()))
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Duration:   %-50s │\n", totalDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	if policyID != "" {
		fmt.Println("Monitor policy execution:")
		fmt.Printf("  vcli policy status %s        # Check status and next execution time\n", policyID)
		fmt.Printf("  vcli policy transactions %s  # View executed transactions\n", policyID)
		fmt.Printf("  vcli policy history %s       # View transaction history\n", policyID)
	}

	return nil
}

func getPluginServerURL(verifierURL, pluginID string) (string, error) {
	return GetPluginServerURL(pluginID)
}

func getPluginPolicySuggest(pluginServerURL string, recipeConfig map[string]interface{}) (*rtypes.PolicySuggest, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"configuration": recipeConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", pluginServerURL+"/plugin/recipe-specification/suggest", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("suggest failed (%d): %s", resp.StatusCode, string(body))
	}

	policySuggest := &rtypes.PolicySuggest{}
	err = protojson.Unmarshal(body, policySuggest)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return policySuggest, nil
}

func buildProtobufPolicy(pluginID string, recipeConfig map[string]interface{}, billingConfig interface{}, suggest *rtypes.PolicySuggest) (*rtypes.Policy, error) {
	// Build Configuration from recipe config
	configuration, err := structpb.NewStruct(recipeConfig)
	if err != nil {
		return nil, fmt.Errorf("build configuration struct: %w", err)
	}

	// Build FeePolicies from billing config
	feePolicies, err := buildFeePolicies(billingConfig)
	if err != nil {
		return nil, fmt.Errorf("build fee policies: %w", err)
	}

	// Policy ID must match the plugin ID for schema validation
	policy := &rtypes.Policy{
		Id:            pluginID,
		Configuration: configuration,
		Rules:         suggest.GetRules(),
		FeePolicies:   feePolicies,
	}

	if suggest.RateLimitWindow != nil {
		policy.RateLimitWindow = suggest.RateLimitWindow
	}
	if suggest.MaxTxsPerWindow != nil {
		policy.MaxTxsPerWindow = suggest.MaxTxsPerWindow
	}

	return policy, nil
}

func buildFeePolicies(billingConfig interface{}) ([]*rtypes.FeePolicy, error) {
	if billingConfig == nil {
		return nil, nil
	}

	var billingArray []interface{}

	switch v := billingConfig.(type) {
	case []interface{}:
		billingArray = v
	case map[string]interface{}:
		billingArray = []interface{}{v}
	default:
		return nil, fmt.Errorf("invalid billing config type: %T", billingConfig)
	}

	var feePolicies []*rtypes.FeePolicy
	for _, item := range billingArray {
		billing, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		feePolicy := &rtypes.FeePolicy{
			Id: uuid.New().String(),
		}

		if typeStr, ok := billing["type"].(string); ok {
			switch strings.ToLower(typeStr) {
			case "once", "one_time", "one-time":
				feePolicy.Type = rtypes.FeeType_ONCE
			case "transaction", "per_tx", "per-tx":
				feePolicy.Type = rtypes.FeeType_TRANSACTION
			case "recurring":
				feePolicy.Type = rtypes.FeeType_RECURRING
			default:
				feePolicy.Type = rtypes.FeeType_ONCE
			}
		}

		if amount, ok := billing["amount"].(float64); ok {
			feePolicy.Amount = int64(amount)
		}

		if freq, ok := billing["frequency"].(string); ok {
			switch strings.ToLower(freq) {
			case "daily":
				feePolicy.Frequency = rtypes.BillingFrequency_DAILY
			case "weekly":
				feePolicy.Frequency = rtypes.BillingFrequency_WEEKLY
			case "biweekly", "bi-weekly":
				feePolicy.Frequency = rtypes.BillingFrequency_BIWEEKLY
			case "monthly":
				feePolicy.Frequency = rtypes.BillingFrequency_MONTHLY
			}
		}

		feePolicies = append(feePolicies, feePolicy)
	}

	return feePolicies, nil
}

func buildBillingArray(billingConfig interface{}) ([]map[string]interface{}, error) {
	if billingConfig == nil {
		return []map[string]interface{}{}, nil
	}

	var billingArray []interface{}

	switch v := billingConfig.(type) {
	case []interface{}:
		billingArray = v
	case map[string]interface{}:
		billingArray = []interface{}{v}
	default:
		return nil, fmt.Errorf("invalid billing config type: %T", billingConfig)
	}

	var result []map[string]interface{}
	for _, item := range billingArray {
		if billing, ok := item.(map[string]interface{}); ok {
			result = append(result, billing)
		}
	}

	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runPolicyDelete(policyID, password string) error {
	startTime := time.Now()

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w\n\nRun 'vcli vault import' first", err)
	}

	vaults, err := ListVaults()
	if err != nil || len(vaults) == 0 {
		return fmt.Errorf("no vaults found. Import a vault first")
	}
	vault := vaults[0]

	fmt.Printf("Deleting policy %s...\n", policyID)
	fmt.Printf("  Vault: %s...\n", vault.PublicKeyECDSA[:20])

	// Step 1: Fetch existing policy to get its data
	fmt.Println("\nFetching policy details...")
	policyURL := fmt.Sprintf("%s/plugin/policy/%s", cfg.Verifier, policyID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fetchReq, err := http.NewRequestWithContext(ctx, "GET", policyURL, nil)
	if err != nil {
		return fmt.Errorf("create fetch request: %w", err)
	}
	fetchReq.Header.Set("Authorization", authHeader)

	fetchResp, err := http.DefaultClient.Do(fetchReq)
	if err != nil {
		return fmt.Errorf("fetch policy failed: %w", err)
	}
	defer fetchResp.Body.Close()

	fetchBody, _ := io.ReadAll(fetchResp.Body)
	if fetchResp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch policy failed (%d): %s", fetchResp.StatusCode, string(fetchBody))
	}

	var policyResp struct {
		Data struct {
			Recipe        string `json:"recipe"`
			PublicKey     string `json:"public_key"`
			PolicyVersion int    `json:"policy_version"`
			PluginVersion string `json:"plugin_version"`
		} `json:"data"`
	}
	err = json.Unmarshal(fetchBody, &policyResp)
	if err != nil {
		return fmt.Errorf("parse policy response: %w", err)
	}

	// Step 2: Reconstruct signature message using policy data
	// Message format: {recipe}*#*{public_key}*#*{policy_version}*#*{plugin_version}
	signatureMessage := fmt.Sprintf("%s*#*%s*#*%d*#*%s",
		policyResp.Data.Recipe,
		policyResp.Data.PublicKey,
		policyResp.Data.PolicyVersion,
		policyResp.Data.PluginVersion,
	)

	fmt.Printf("  Policy Version: %d\n", policyResp.Data.PolicyVersion)
	fmt.Printf("  Plugin Version: %s\n", policyResp.Data.PluginVersion)

	ethPrefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(signatureMessage), signatureMessage)
	messageHash := crypto.Keccak256([]byte(ethPrefixedMessage))
	hexMessage := hex.EncodeToString(messageHash)
	fmt.Printf("  Message hash: %s\n", hexMessage)

	// Step 3: Sign with TSS keysign
	fmt.Println("\nSigning deletion with TSS keysign (2-of-2 with Fast Vault Server)...")

	tss := NewTSSService(vault.LocalPartyID)
	signCtx, signCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer signCancel()

	derivePath := "m/44'/60'/0'/0/0"
	results, err := tss.KeysignWithFastVault(signCtx, vault, []string{hexMessage}, derivePath, password)
	if err != nil {
		return fmt.Errorf("TSS keysign failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no signature result")
	}

	signature := "0x" + results[0].R + results[0].S + results[0].RecoveryID
	fmt.Printf("  Signature: %s...\n", signature[:20])

	// Step 4: Send DELETE request with signature
	fmt.Println("\nDeleting policy...")

	deleteBody := map[string]string{
		"signature": signature,
	}
	deleteJSON, err := json.Marshal(deleteBody)
	if err != nil {
		return fmt.Errorf("marshal delete body: %w", err)
	}

	deleteReq, err := http.NewRequestWithContext(ctx, "DELETE", policyURL, bytes.NewReader(deleteJSON))
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.Header.Set("Authorization", authHeader)

	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer deleteResp.Body.Close()

	deleteRespBody, _ := io.ReadAll(deleteResp.Body)

	if deleteResp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete policy failed (%d): %s", deleteResp.StatusCode, string(deleteRespBody))
	}

	totalDuration := time.Since(startTime)

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ POLICY DELETED SUCCESSFULLY                                     │")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Policy ID:      %-47s │\n", policyID)
	fmt.Printf("│  Vault:          %-47s │\n", vault.PublicKeyECDSA[:16]+"...")
	fmt.Printf("│  Policy Version: %-47d │\n", policyResp.Data.PolicyVersion)
	fmt.Printf("│  Plugin Version: %-47s │\n", policyResp.Data.PluginVersion)
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Duration:       %-47s │\n", totalDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Next: vcli policy list --plugin <plugin-id>   # Check remaining policies")
	fmt.Println("      vcli plugin uninstall <plugin-id>       # When done testing")

	return nil
}

func runPolicyInfo(policyID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return fmt.Errorf("authentication required: %w\n\nRun 'vcli vault import' first", err)
	}

	fmt.Printf("Fetching policy %s...\n\n", policyID)

	url := fmt.Sprintf("%s/plugin/policy/%s", cfg.Verifier, policyID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
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

func runPolicyHistory(policyID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Fetching transaction history for policy %s...\n\n", policyID)

	url := fmt.Sprintf("%s/plugin/policies/%s/history", cfg.Verifier, policyID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func fillAddressesFromVault(recipeConfig map[string]interface{}, vault *LocalVault) (map[string]interface{}, error) {
	fromAsset, hasFrom := recipeConfig["from"].(map[string]interface{})
	toAsset, hasTo := recipeConfig["to"].(map[string]interface{})

	if !hasFrom && !hasTo {
		return recipeConfig, nil
	}

	deriveAddress := func(chainStr string) (string, error) {
		chain, err := common.FromString(chainStr)
		if err != nil {
			return "", fmt.Errorf("unknown chain: %s", chainStr)
		}

		pubKey := vault.PublicKeyECDSA
		if chain == common.Solana {
			pubKey = vault.PublicKeyEdDSA
		}

		addr, _, _, err := address.GetAddress(pubKey, vault.HexChainCode, chain)
		if err != nil {
			return "", fmt.Errorf("derive address for %s: %w", chainStr, err)
		}
		return addr, nil
	}

	if hasFrom {
		chainStr, _ := fromAsset["chain"].(string)
		existingAddr, _ := fromAsset["address"].(string)
		if existingAddr == "" && chainStr != "" {
			addr, err := deriveAddress(chainStr)
			if err != nil {
				return nil, err
			}
			fromAsset["address"] = addr
			fmt.Printf("  Auto-filled from.address: %s\n", addr)
		}
	}

	if hasTo {
		chainStr, _ := toAsset["chain"].(string)
		existingAddr, _ := toAsset["address"].(string)
		if existingAddr == "" && chainStr != "" {
			addr, err := deriveAddress(chainStr)
			if err != nil {
				return nil, err
			}
			toAsset["address"] = addr
			fmt.Printf("  Auto-filled to.address: %s\n", addr)
		}
	}

	return recipeConfig, nil
}

func newPolicyStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [policy-id]",
		Short: "Show policy status including scheduler info",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyStatus(args[0])
		},
	}
	return cmd
}

func newPolicyTransactionsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "transactions [policy-id]",
		Short: "Show transactions for a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyTransactions(args[0], limit)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Number of transactions to show")
	return cmd
}

func newPolicyTriggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trigger [policy-id]",
		Short: "Manually trigger policy execution (set next_execution = NOW)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyTrigger(args[0])
		},
	}
	return cmd
}

func runPolicyStatus(policyID string) error {
	fmt.Printf("Policy Status: %s\n", policyID)
	fmt.Println(strings.Repeat("=", 50))

	policyActive, policyCreated := checkPolicyInDB(policyID)
	fmt.Printf("\nPolicy Record:\n")
	if policyCreated != "" {
		fmt.Printf("  Active:  %v\n", policyActive)
		fmt.Printf("  Created: %s\n", policyCreated)
	} else {
		fmt.Printf("  ✗ Not found in database\n")
	}

	nextExec := checkScheduler(policyID)
	fmt.Printf("\nScheduler:\n")
	if nextExec != "" {
		fmt.Printf("  Next Execution: %s\n", nextExec)
	} else {
		fmt.Printf("  ✗ Not scheduled (policy may be inactive or one-time completed)\n")
	}

	fmt.Printf("\nRecent Transactions:\n")
	txs := getRecentTransactions(policyID, 3)
	if len(txs) == 0 {
		fmt.Printf("  No transactions found\n")
	} else {
		for i, tx := range txs {
			fmt.Printf("\n  Transaction %d:\n", i+1)
			fmt.Printf("    TX Hash:    %s\n", tx.TxHash)
			fmt.Printf("    Status:     %s\n", tx.Status)
			fmt.Printf("    On-chain:   %s\n", tx.OnChainStatus)
			fmt.Printf("    Created:    %s\n", tx.CreatedAt)
			if tx.TxHash != "" && tx.TxHash != "<nil>" && tx.TxHash != "NULL" {
				if explorerURL := getExplorerURL(policyID, tx.TxHash); explorerURL != "" {
					fmt.Printf("    Explorer:   %s\n", explorerURL)
				}
			}
		}
		fmt.Println()
	}

	return nil
}

func getExplorerURL(policyID, txHash string) string {
	// Try to fetch policy to determine chain
	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}

	authHeader, err := GetAuthHeader()
	if err != nil {
		return ""
	}

	policyURL := fmt.Sprintf("%s/plugin/policy/%s", cfg.Verifier, policyID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", policyURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	var policyResp struct {
		Data struct {
			Recipe string `json:"recipe"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &policyResp)

	// Decode recipe to get chain info
	if policyResp.Data.Recipe != "" {
		recipeBytes, err := base64.StdEncoding.DecodeString(policyResp.Data.Recipe)
		if err == nil {
			// Try to parse as JSON to extract chain
			var recipe map[string]interface{}
			if json.Unmarshal(recipeBytes, &recipe) == nil {
				if from, ok := recipe["from"].(map[string]interface{}); ok {
					if chain, ok := from["chain"].(string); ok {
						return getExplorerURLForChain(chain, txHash)
					}
				}
			}
		}
	}

	return ""
}

func getExplorerURLForChain(chain, txHash string) string {
	chainLower := strings.ToLower(chain)
	switch chainLower {
	case "ethereum", "eth":
		return fmt.Sprintf("https://etherscan.io/tx/%s", txHash)
	case "arbitrum", "arb":
		return fmt.Sprintf("https://arbiscan.io/tx/%s", txHash)
	case "base":
		return fmt.Sprintf("https://basescan.org/tx/%s", txHash)
	case "polygon", "matic":
		return fmt.Sprintf("https://polygonscan.com/tx/%s", txHash)
	case "bsc", "bnb", "binance":
		return fmt.Sprintf("https://bscscan.com/tx/%s", txHash)
	case "avalanche", "avax":
		return fmt.Sprintf("https://snowtrace.io/tx/%s", txHash)
	case "optimism", "op":
		return fmt.Sprintf("https://optimistic.etherscan.io/tx/%s", txHash)
	case "bitcoin", "btc":
		return fmt.Sprintf("https://blockstream.info/tx/%s", txHash)
	case "solana", "sol":
		return fmt.Sprintf("https://solscan.io/tx/%s", txHash)
	default:
		return ""
	}
}

func runPolicyTransactions(policyID string, limit int) error {
	fmt.Printf("Transactions for Policy: %s\n", policyID)
	fmt.Println(strings.Repeat("=", 60))

	txs := getRecentTransactions(policyID, limit)
	if len(txs) == 0 {
		fmt.Println("\nNo transactions found for this policy.")
		fmt.Println("\nPossible reasons:")
		fmt.Println("  - Policy hasn't executed yet (check scheduler)")
		fmt.Println("  - Policy is inactive")
		fmt.Println("  - Scheduler hasn't picked it up (polls every 30s)")
		return nil
	}

	fmt.Printf("\nFound %d transactions:\n\n", len(txs))
	for i, tx := range txs {
		fmt.Printf("┌─────────────────────────────────────────────────────────────────┐\n")
		fmt.Printf("│ Transaction %d                                                  │\n", i+1)
		fmt.Printf("├─────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│                                                                 │\n")
		fmt.Printf("│  TX Hash:     %s\n", tx.TxHash)
		fmt.Printf("│                                                                 │\n")
		fmt.Printf("│  Status:      %-52s │\n", tx.Status)
		fmt.Printf("│  On-chain:    %-52s │\n", tx.OnChainStatus)
		fmt.Printf("│  Created:     %-52s │\n", tx.CreatedAt)
		if tx.TxHash != "" && tx.TxHash != "<nil>" && tx.TxHash != "NULL" {
			if explorerURL := getExplorerURL(policyID, tx.TxHash); explorerURL != "" {
				fmt.Printf("│                                                                 │\n")
				fmt.Printf("│  Explorer:   %s\n", explorerURL)
			}
		}
		fmt.Printf("│                                                                 │\n")
		fmt.Printf("└─────────────────────────────────────────────────────────────────┘\n")
		fmt.Println()
	}

	return nil
}

func runPolicyTrigger(policyID string) error {
	fmt.Printf("Triggering policy: %s\n", policyID)

	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-dca", "-c",
		fmt.Sprintf("UPDATE scheduler SET next_execution = NOW() WHERE policy_id = '%s'", policyID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update scheduler: %w\nOutput: %s", err, string(output))
	}

	result := strings.TrimSpace(string(output))
	if strings.Contains(result, "UPDATE 0") {
		fmt.Println("⚠ Policy not found in scheduler table.")
		fmt.Println("  This might mean:")
		fmt.Println("  - Policy doesn't exist")
		fmt.Println("  - Policy is inactive (one-time completed)")
		fmt.Println("  - Policy hasn't been scheduled yet")
		return nil
	}

	fmt.Println("✓ Policy triggered! Scheduler will pick it up within 30 seconds.")
	fmt.Println("\nMonitor with:")
	fmt.Println("  vcli policy status " + policyID)
	fmt.Println("  vcli policy transactions " + policyID)

	return nil
}

type TxRecord struct {
	TxHash        string
	Status        string
	OnChainStatus string
	CreatedAt     string
}

func checkPolicyInDB(policyID string) (bool, string) {
	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-verifier", "-t", "-c",
		fmt.Sprintf("SELECT active, created_at FROM plugin_policies WHERE id = '%s' LIMIT 1", policyID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, ""
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return false, ""
	}

	parts := strings.Split(result, "|")
	if len(parts) < 2 {
		return false, ""
	}

	active := strings.TrimSpace(parts[0]) == "t"
	created := strings.TrimSpace(parts[1])
	return active, created
}

func checkScheduler(policyID string) string {
	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-dca", "-t", "-c",
		fmt.Sprintf("SELECT next_execution FROM scheduler WHERE policy_id = '%s' LIMIT 1", policyID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

func getRecentTransactions(policyID string, limit int) []TxRecord {
	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-dca", "-t", "-c",
		fmt.Sprintf(`SELECT tx_hash, status, status_onchain, created_at
			FROM tx_indexer
			WHERE policy_id = '%s'
			ORDER BY created_at DESC
			LIMIT %d`, policyID, limit))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var txs []TxRecord
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		txs = append(txs, TxRecord{
			TxHash:        strings.TrimSpace(parts[0]),
			Status:        strings.TrimSpace(parts[1]),
			OnChainStatus: strings.TrimSpace(parts[2]),
			CreatedAt:     strings.TrimSpace(parts[3]),
		})
	}

	return txs
}
