package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func NewPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Plugin management commands",
	}

	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginAliasesCmd())
	cmd.AddCommand(newPluginInfoCmd())
	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginUninstallCmd())
	cmd.AddCommand(newPluginSpecCmd())
	cmd.AddCommand(newPluginInstalledCmd())

	return cmd
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available plugins from verifier",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginList()
		},
	}
}

func newPluginAliasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "aliases",
		Short: "Show plugin aliases (short names)",
		Long: `Show available plugin aliases for convenience.

You can use either the short alias or the full plugin ID in any command.

Examples:
  vcli plugin install dca              # Uses alias
  vcli plugin install vultisig-dca-0000  # Uses full ID
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginAliases()
		},
	}
}

func runPluginAliases() error {
	fmt.Println("Available Plugin Aliases:")
	fmt.Println()
	fmt.Println("┌──────────┬─────────────────────────────────┬────────────────────────────────┐")
	fmt.Println("│ Alias    │ Full Plugin ID                  │ Description                    │")
	fmt.Println("├──────────┼─────────────────────────────────┼────────────────────────────────┤")
	for _, p := range KnownPlugins {
		aliases := strings.Join(p.Aliases, ", ")
		name := p.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		fmt.Printf("│ %-8s │ %-31s │ %-30s │\n", aliases, p.ID, name)
	}
	fmt.Println("└──────────┴─────────────────────────────────┴────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Usage: vcli plugin install <alias-or-full-id>")
	fmt.Println("       vcli policy add --plugin <alias-or-full-id> ...")
	return nil
}

func newPluginInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [plugin-id]",
		Short: "Show plugin details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInfo(args[0])
		},
	}
}

func newPluginInstallCmd() *cobra.Command {
	var password string

	cmd := &cobra.Command{
		Use:   "install [plugin-id]",
		Short: "Install a plugin (initiates reshare)",
		Long: `Install a plugin by initiating a TSS reshare operation.

This will:
1. Check if the plugin exists and is available
2. Initiate a reshare session to add the plugin as a signer
3. Wait for the TSS session to complete

After installation, you can create policies for the plugin.

Environment variables:
  VAULT_PASSWORD  - Fast Vault password

Note: Requires authentication. Run 'vcli vault import' first.
`,
		Args: cobra.ExactArgs(1),
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
			return runPluginInstall(args[0], actualPassword)
		},
	}

	cmd.Flags().StringVar(&password, "password", "", "Fast Vault password (or set VAULT_PASSWORD env var)")

	return cmd
}

func newPluginUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [plugin-id]",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginUninstall(args[0])
		},
	}
}

func newPluginSpecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spec [plugin-id]",
		Short: "Show plugin recipe specification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginSpec(args[0])
		},
	}
}

func runPluginList() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Println("Fetching available plugins...")

	url := cfg.Verifier + "/plugins"
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

	if data, ok := result["data"].(map[string]interface{}); ok {
		if plugins, ok := data["plugins"].([]interface{}); ok {
			fmt.Printf("\nAvailable Plugins (%d):\n\n", len(plugins))
			for _, p := range plugins {
				plugin := p.(map[string]interface{})
				fmt.Printf("  %s\n", plugin["id"])
				fmt.Printf("    Name: %s\n", plugin["title"])
				if desc, ok := plugin["description"].(string); ok && desc != "" {
					fmt.Printf("    Description: %s\n", desc)
				}
				fmt.Println()
			}
			return nil
		}
	}

	prettyJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(prettyJSON))

	return nil
}

func runPluginInfo(pluginID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Fetching plugin info for %s...\n\n", pluginID)

	url := fmt.Sprintf("%s/plugins/%s", cfg.Verifier, pluginID)
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

func runPluginInstall(pluginIDOrAlias string, password string) error {
	startTime := time.Now()

	// Resolve alias to full plugin ID
	pluginID := ResolvePluginID(pluginIDOrAlias)
	if pluginID != pluginIDOrAlias {
		fmt.Printf("Resolved alias '%s' to plugin ID: %s\n", pluginIDOrAlias, pluginID)
	}

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	vaults, err := ListVaults()
	if err != nil || len(vaults) == 0 {
		return fmt.Errorf("no vaults found. Import a vault first: vcli vault import")
	}
	vault := vaults[0]

	authHeader, err := GetAuthHeader()
	if err != nil {
		if password == "" {
			return fmt.Errorf("authentication required: %w\n\nRun 'vcli vault import --password xxx' to authenticate first", err)
		}
		fmt.Println("No valid auth token found; authenticating with Fast Vault...")
		if authErr := authenticateVault(vault, password); authErr != nil {
			return fmt.Errorf("authentication required: %w\n\nautomatic authentication failed: %v", err, authErr)
		}
		authHeader, err = GetAuthHeader()
		if err != nil {
			return fmt.Errorf("authentication required after re-auth: %w", err)
		}
	}

	fmt.Printf("Installing plugin %s...\n", pluginID)
	fmt.Printf("  Vault: %s (%s...)\n", vault.Name, vault.PublicKeyECDSA[:16])
	fmt.Printf("  Verifier: %s\n", cfg.Verifier)

	isFastVault, err := CheckFastVaultExists(vault.PublicKeyECDSA)
	if err != nil {
		fmt.Printf("  Warning: Could not check Fast Vault Server: %v\n", err)
	} else if !isFastVault {
		return fmt.Errorf("vault is not a Fast Vault. Plugin reshare requires a vault created with Fast Vault feature")
	} else {
		fmt.Println("  Fast Vault: Yes")
	}

	if password == "" {
		return fmt.Errorf("password is required for Fast Vault reshare. Use --password flag")
	}

	isProduction := strings.Contains(cfg.Verifier, "vultisig.com")

	var isInstalled bool
	var dbRecord string
	if isProduction {
		installed, err := checkPluginInstallationProduction(cfg, pluginID, vault.PublicKeyECDSA)
		if err != nil {
			fmt.Printf("  Warning: Could not check installation status: %v\n", err)
		}
		isInstalled = installed
	} else {
		dbRecord = checkPluginInstallation(pluginID, vault.PublicKeyECDSA)
		isInstalled = dbRecord != ""
	}

	if isInstalled {
		fmt.Printf("\n  Plugin %s is already installed for this vault.\n", pluginID)
		if dbRecord != "" {
			fmt.Printf("  Installed at: %s\n", dbRecord)
		}
		fmt.Println("\n  To reinstall, first run: vcli plugin uninstall", pluginID)
		return nil
	}

	fmt.Println("\nChecking plugin availability...")
	pluginURL := fmt.Sprintf("%s/plugins/%s", cfg.Verifier, pluginID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", pluginURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("check plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plugin not found: %s", string(body))
	}

	fmt.Println("  Plugin found!")

	fmt.Println("\nInitiating 4-party TSS reshare...")
	fmt.Println("  Parties: CLI + Fast Vault Server + Verifier + Plugin")

	tss := NewTSSService(vault.LocalPartyID)

	reshareStart := time.Now()
	reshareCtx, reshareCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer reshareCancel()

	newVault, err := tss.ReshareWithDKLS(reshareCtx, vault, pluginID, cfg.Verifier, authHeader, password)
	if err != nil {
		return fmt.Errorf("reshare failed: %w", err)
	}
	reshareDuration := time.Since(reshareStart)

	// NOTE: We intentionally do NOT save the 4-party vault locally.
	// The plugin's 2-of-4 keyshares are stored in MinIO by verifier and worker.
	// The local vault remains at 2-of-2 so users can:
	// 1. Sign transactions directly with CLI + Fast Vault Server
	// 2. Install additional plugins from the same original vault
	_ = newVault // Keyshare is uploaded to MinIO, not saved locally

	totalDuration := time.Since(startTime)

	// Wait for workers to upload keyshares to MinIO
	fmt.Println("\nWaiting for keyshare uploads...")
	time.Sleep(3 * time.Second)

	// Validate storage - check MinIO buckets (with retry)
	verifierFile, verifierSize := checkMinioFileWithRetry("vultisig-verifier", pluginID, vault.PublicKeyECDSA, 3)
	pluginBucket := getPluginBucket(pluginID)
	pluginFile, pluginSize := checkMinioFileWithRetry(pluginBucket, pluginID, vault.PublicKeyECDSA, 3)

	// Check database record
	dbRecord = checkPluginInstallation(pluginID, vault.PublicKeyECDSA)

	// Print completion report
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PLUGIN INSTALL COMPLETE                                         │")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Plugin:     %-50s │\n", pluginID)
	fmt.Printf("│  Vault:      %-50s │\n", vault.PublicKeyECDSA[:16]+"...")
	fmt.Println("│                                                                 │")
	fmt.Println("│  Summary:                                                      │")
	fmt.Printf("│    Threshold: %-47s │\n", fmt.Sprintf("%d-of-%d", 2, len(newVault.Signers)))
	fmt.Printf("│    Parties:   %-47d │\n", len(newVault.Signers))
	fmt.Println("│                                                                 │")
	fmt.Println("│  TSS Reshare:                                                   │")
	for i, signer := range newVault.Signers {
		role := getSignerRole(signer, vault.LocalPartyID)
		signerDisplay := signer
		if len(signerDisplay) > 25 {
			signerDisplay = signerDisplay[:25] + ".."
		}
		fmt.Printf("│      %d. %-27s %-17s │\n", i+1, signerDisplay, role)
	}
	fmt.Printf("│    Duration: %-50s │\n", reshareDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("│  Keyshares Stored:                                              │")
	if verifierFile != "" {
		fmt.Printf("│    Verifier (MinIO): ✓ %-41s │\n", verifierSize)
	} else {
		fmt.Printf("│    Verifier (MinIO): ✗ %-41s │\n", "Not found")
	}
	pluginLabel := getPluginLabel(pluginID)
	if pluginFile != "" {
		fmt.Printf("│    %s (MinIO): ✓ %-38s │\n", pluginLabel, pluginSize)
	} else {
		fmt.Printf("│    %s (MinIO): ✗ %-38s │\n", pluginLabel, "Not found")
	}
	fmt.Println("│                                                                 │")
	fmt.Println("│  Database:                                                      │")
	if dbRecord != "" {
		fmt.Printf("│    plugin_installations: ✓ %-37s │\n", dbRecord)
	} else {
		fmt.Printf("│    plugin_installations: ✗ %-37s │\n", "Not found")
	}
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Total Time: %-51s │\n", totalDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Next: vcli policy generate --from <asset> --to <asset> --amount <amount> --output policy.json")
	fmt.Println("      vcli policy add --plugin", pluginID, "--policy-file policy.json --password <password>")

	return nil
}

func getSignerRole(signer, localPartyID string) string {
	if signer == localPartyID {
		return "(CLI)"
	}
	if strings.HasPrefix(signer, "Server-") {
		return "(Fast Vault Server)"
	}
	if strings.HasPrefix(signer, "verifier-") {
		return "(Verifier)"
	}
	if strings.HasPrefix(signer, "dca-worker-") {
		return "(DCA Plugin)"
	}
	if strings.HasPrefix(signer, "sends-worker-") {
		return "(Sends Plugin)"
	}
	return ""
}

func getPluginBucket(pluginID string) string {
	if strings.Contains(pluginID, "send") {
		return "vultisig-sends"
	}
	return "vultisig-dca"
}

func getPluginLabel(pluginID string) string {
	if strings.Contains(pluginID, "send") {
		return "Sends Plugin"
	}
	return "DCA Plugin  "
}

func checkMinioFileWithRetry(bucket, pluginID, publicKey string, maxRetries int) (string, string) {
	for i := 0; i < maxRetries; i++ {
		file, size := checkMinioFile(bucket, pluginID, publicKey)
		if file != "" {
			return file, size
		}
		if i < maxRetries-1 {
			time.Sleep(time.Second)
		}
	}
	return "", ""
}

func checkMinioFile(bucket, pluginID, publicKey string) (string, string) {
	fileName := fmt.Sprintf("%s-%s.vult", pluginID, publicKey)
	// Set up alias and check file in one command
	cmd := exec.Command("docker", "exec", "vultisig-minio",
		"/bin/sh", "-c",
		fmt.Sprintf("mc alias set myminio http://localhost:9000 minioadmin minioadmin >/dev/null 2>&1 && mc ls --json myminio/%s/%s", bucket, fileName))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", ""
	}

	var obj struct {
		Key  string `json:"key"`
		Size int64  `json:"size"`
	}
	json.Unmarshal(output, &obj)

	if obj.Key != "" {
		size := formatBytesShort(obj.Size)
		return obj.Key, size
	}
	return "", ""
}

func formatBytesShort(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

func checkPluginInstallation(pluginID, publicKey string) string {
	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-verifier", "-t", "-c",
		fmt.Sprintf("SELECT installed_at FROM plugin_installations WHERE plugin_id='%s' AND public_key='%s' LIMIT 1", pluginID, publicKey))

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return ""
	}

	t, err := time.Parse("2006-01-02 15:04:05.999999-07", result)
	if err != nil {
		return result
	}
	return t.Format("2006-01-02 15:04:05")
}

func checkPluginInstallationProduction(cfg *DevConfig, pluginID, publicKey string) (bool, error) {
	url := fmt.Sprintf("%s/vault/exist/%s/%s", cfg.Verifier, pluginID, publicKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func runPluginUninstall(pluginID string) error {
	startTime := time.Now()

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.PublicKeyECDSA == "" {
		return fmt.Errorf("no vault configured. Run 'vcli vault import' first")
	}

	fmt.Printf("Uninstalling plugin %s...\n", pluginID)
	fmt.Printf("  Vault: %s\n", cfg.PublicKeyECDSA[:16]+"...")

	isProduction := strings.Contains(cfg.Verifier, "vultisig.com")

	if isProduction {
		return runPluginUninstallProduction(cfg, pluginID, startTime)
	}

	return runPluginUninstallLocal(cfg, pluginID, startTime)
}

func runPluginUninstallProduction(cfg *DevConfig, pluginID string, startTime time.Time) error {
	if cfg.AuthToken == "" {
		return fmt.Errorf("not authenticated. Run 'vcli auth login' first")
	}

	fmt.Printf("  Verifier: %s\n", cfg.Verifier)

	fmt.Println("\nRemoving plugin via verifier API...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/plugin/%s", cfg.Verifier, pluginID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	totalDuration := time.Since(startTime)

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PLUGIN UNINSTALL                                                │")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Plugin:    %-52s │\n", pluginID)
	fmt.Printf("│  Vault:     %-52s │\n", cfg.PublicKeyECDSA[:16]+"...")
	fmt.Println("│                                                                 │")

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		fmt.Printf("│  Status:    ✓ %-50s │\n", "Uninstalled successfully")
	} else if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("│  Status:    - %-50s │\n", "Plugin was not installed")
	} else {
		fmt.Printf("│  Status:    ✗ %-50s │\n", fmt.Sprintf("Failed (%d)", resp.StatusCode))
		fmt.Printf("│  Response:  %-52s │\n", truncateString(string(body), 50))
	}

	fmt.Println("│                                                                 │")
	fmt.Printf("│  Total Time: %-51s │\n", totalDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Next: vcli plugin install", pluginID, "--password <password>")

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runPluginUninstallLocal(cfg *DevConfig, pluginID string, startTime time.Time) error {
	// Check current installation status
	pluginBucket := getPluginBucket(pluginID)
	dbRecord := checkPluginInstallation(pluginID, cfg.PublicKeyECDSA)
	verifierFile, _ := checkMinioFile("vultisig-verifier", pluginID, cfg.PublicKeyECDSA)
	pluginFile, _ := checkMinioFile(pluginBucket, pluginID, cfg.PublicKeyECDSA)

	if dbRecord == "" && verifierFile == "" && pluginFile == "" {
		fmt.Println("\n  Plugin is not installed for this vault.")
		return nil
	}

	fmt.Println("\nRemoving plugin data...")

	// Remove MinIO files (verifier + plugin 2-of-4 shares)
	verifierRemoved := removeMinioFile("vultisig-verifier", pluginID, cfg.PublicKeyECDSA)
	pluginRemoved := removeMinioFile(pluginBucket, pluginID, cfg.PublicKeyECDSA)

	// Remove database record
	dbRemoved := removePluginInstallation(pluginID, cfg.PublicKeyECDSA)

	totalDuration := time.Since(startTime)

	// Print completion report
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PLUGIN UNINSTALL COMPLETE                                       │")
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Plugin:    %-52s │\n", pluginID)
	fmt.Printf("│  Vault:     %-52s │\n", cfg.PublicKeyECDSA[:16]+"...")
	fmt.Println("│                                                                 │")
	fmt.Println("│  Removed:                                                       │")
	if verifierRemoved {
		fmt.Printf("│    Verifier keyshare (MinIO): ✓ %-32s │\n", "Deleted")
	} else if verifierFile != "" {
		fmt.Printf("│    Verifier keyshare (MinIO): ✗ %-32s │\n", "Failed to delete")
	} else {
		fmt.Printf("│    Verifier keyshare (MinIO): - %-32s │\n", "Not found")
	}
	pluginLabel := getPluginLabel(pluginID)
	if pluginRemoved {
		fmt.Printf("│    %s keyshare (MinIO): ✓ %-29s │\n", pluginLabel, "Deleted")
	} else if pluginFile != "" {
		fmt.Printf("│    %s keyshare (MinIO): ✗ %-29s │\n", pluginLabel, "Failed to delete")
	} else {
		fmt.Printf("│    %s keyshare (MinIO): - %-29s │\n", pluginLabel, "Not found")
	}
	if dbRemoved {
		fmt.Printf("│    Database record: ✓ %-42s │\n", "Deleted")
	} else if dbRecord != "" {
		fmt.Printf("│    Database record: ✗ %-42s │\n", "Failed to delete")
	} else {
		fmt.Printf("│    Database record: - %-42s │\n", "Not found")
	}
	fmt.Println("│                                                                 │")
	fmt.Printf("│  Total Time: %-51s │\n", totalDuration.Round(time.Millisecond).String())
	fmt.Println("│                                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Next: make stop                               # Stop all services")
	fmt.Println("      vcli plugin install", pluginID, "-p   # Or reinstall plugin")

	return nil
}

func removeMinioFile(bucket, pluginID, publicKey string) bool {
	fileName := fmt.Sprintf("%s-%s.vult", pluginID, publicKey)
	cmd := exec.Command("docker", "run", "--rm", "--network", "devenv_vultisig",
		"-e", "MC_HOST_minio=http://minioadmin:minioadmin@vultisig-minio:9000",
		"minio/mc", "rm", "minio/"+bucket+"/"+fileName)

	err := cmd.Run()
	return err == nil
}

func removePluginInstallation(pluginID, publicKey string) bool {
	cmd := exec.Command("docker", "exec", "vultisig-postgres",
		"psql", "-U", "vultisig", "-d", "vultisig-verifier", "-c",
		fmt.Sprintf("DELETE FROM plugin_installations WHERE plugin_id='%s' AND public_key='%s'", pluginID, publicKey))

	err := cmd.Run()
	return err == nil
}

func runPluginSpec(pluginID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Fetching recipe specification for %s...\n\n", pluginID)

	url := fmt.Sprintf("%s/plugins/%s/recipe-specification", cfg.Verifier, pluginID)
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

func doRequest(method, url string, body interface{}) ([]byte, int, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, 0, err
	}
	_ = cfg

	var reqBody io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBody)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

func newPluginInstalledCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "installed",
		Short: "List installed plugins for current vault",
		Long: `List all plugins that have been installed for the current vault.

This queries the verifier to show which plugins have keyshares registered
for your vault's public key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstalled()
		},
	}
}

func runPluginInstalled() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.PublicKeyECDSA == "" {
		return fmt.Errorf("no vault configured. Run 'vcli vault import' first")
	}

	fmt.Printf("Fetching installed plugins for vault %s...\n\n", cfg.PublicKeyECDSA[:16]+"...")

	url := fmt.Sprintf("%s/plugins/installed?public_key=%s", cfg.Verifier, cfg.PublicKeyECDSA)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data struct {
			Plugins []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"plugins"`
			TotalCount int `json:"total_count"`
		} `json:"data"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Status int `json:"status"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if result.Error.Message != "" {
		return fmt.Errorf("verifier error: %s", result.Error.Message)
	}

	if result.Data.TotalCount == 0 {
		fmt.Println("No plugins installed for this vault.")
		fmt.Println("\nTo install a plugin: vcli plugin install <plugin-id> --password <password>")
		return nil
	}

	fmt.Printf("Installed Plugins (%d):\n\n", result.Data.TotalCount)
	fmt.Println("┌─────────────────────────────────────┬────────────────────────────────┐")
	fmt.Println("│ Plugin ID                           │ Name                           │")
	fmt.Println("├─────────────────────────────────────┼────────────────────────────────┤")
	for _, p := range result.Data.Plugins {
		id := p.ID
		if len(id) > 35 {
			id = id[:32] + "..."
		}
		title := p.Title
		if len(title) > 30 {
			title = title[:27] + "..."
		}
		fmt.Printf("│ %-35s │ %-30s │\n", id, title)
	}
	fmt.Println("└─────────────────────────────────────┴────────────────────────────────┘")

	fmt.Println("\nTo view policies: vcli policy list --plugin <plugin-id>")

	return nil
}
