package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type DevConfig struct {
	Verifier       string `json:"verifier_url"`
	FeePlugin      string `json:"fee_plugin_url"`
	DCAPlugin      string `json:"dca_plugin_url"`
	RelayServer    string `json:"relay_server"`
	DatabaseDSN    string `json:"database_dsn"`
	RedisURI       string `json:"redis_uri"`
	MinioHost      string `json:"minio_host"`
	MinioAccess    string `json:"minio_access_key"`
	MinioSecret    string `json:"minio_secret_key"`
	Encryption     string `json:"encryption_secret"`
	VaultName      string `json:"vault_name"`
	PublicKeyECDSA string `json:"public_key_ecdsa"`
	PublicKeyEdDSA string `json:"public_key_eddsa"`
	AuthToken      string `json:"auth_token,omitempty"`
	AuthPublicKey  string `json:"auth_public_key,omitempty"`
	AuthExpiresAt  string `json:"auth_expires_at,omitempty"`
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func DefaultConfig() *DevConfig {
	return &DevConfig{
		Verifier:    getEnvOrDefault("VCLI_VERIFIER_URL", "http://localhost:8080"),
		FeePlugin:   getEnvOrDefault("VCLI_FEE_PLUGIN_URL", "http://localhost:8085"),
		DCAPlugin:   getEnvOrDefault("VCLI_DCA_PLUGIN_URL", "http://localhost:8082"),
		RelayServer: getEnvOrDefault("VCLI_RELAY_URL", "https://api.vultisig.com/router"),
		DatabaseDSN: getEnvOrDefault("VCLI_DATABASE_DSN", "postgres://vultisig:vultisig@localhost:5432/vultisig-verifier?sslmode=disable"),
		RedisURI:    getEnvOrDefault("VCLI_REDIS_URI", "redis://:vultisig@localhost:6379"),
		MinioHost:   getEnvOrDefault("VCLI_MINIO_HOST", "http://localhost:9000"),
		MinioAccess: getEnvOrDefault("VCLI_MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecret: getEnvOrDefault("VCLI_MINIO_SECRET_KEY", "minioadmin"),
		Encryption:  getEnvOrDefault("VCLI_ENCRYPTION_SECRET", "dev-encryption-secret-32b"),
	}
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vultisig", "vcli.json")
}

func LoadConfig() (*DevConfig, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	err = json.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func SaveConfig(cfg *DevConfig) error {
	path := ConfigPath()
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	err = os.WriteFile(path, data, 0600)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// PluginAliases maps short aliases to full plugin IDs.
// Users can use either the alias or the full ID.
var PluginAliases = map[string]string{
	"dca":   "vultisig-dca-0000",
	"fee":   "vultisig-fees-feee",
	"fees":  "vultisig-fees-feee",
	"sends": "vultisig-recurring-sends-0000",
}

// PluginInfo provides details about known plugins for help text
type PluginInfo struct {
	ID          string
	Aliases     []string
	Name        string
	Description string
	ServerURL   string
}

// KnownPlugins lists all plugins with their details
var KnownPlugins = []PluginInfo{
	{
		ID:          "vultisig-dca-0000",
		Aliases:     []string{"dca"},
		Name:        "Recurring Swaps (DCA)",
		Description: "Dollar-cost averaging with automated token swaps",
		ServerURL:   "http://localhost:8082",
	},
	{
		ID:          "vultisig-fees-feee",
		Aliases:     []string{"fee", "fees"},
		Name:        "Vultisig Fees",
		Description: "Fee collection plugin",
		ServerURL:   "http://localhost:8085",
	},
	{
		ID:          "vultisig-recurring-sends-0000",
		Aliases:     []string{"sends"},
		Name:        "Recurring Sends",
		Description: "Automated recurring token transfers",
		ServerURL:   "http://localhost:8083",
	},
}

// ResolvePluginID converts an alias to the full plugin ID.
// If the input is not an alias, it returns the input unchanged.
func ResolvePluginID(input string) string {
	if full, ok := PluginAliases[input]; ok {
		return full
	}
	return input
}

// GetPluginServerURL returns the server URL for a plugin ID (or alias)
func GetPluginServerURL(pluginIDOrAlias string) (string, error) {
	pluginID := ResolvePluginID(pluginIDOrAlias)
	for _, p := range KnownPlugins {
		if p.ID == pluginID {
			return p.ServerURL, nil
		}
	}
	return "", fmt.Errorf("unknown plugin: %s", pluginIDOrAlias)
}

// Asset represents a blockchain asset with chain and optional token address
type Asset struct {
	Chain string
	Token string
}

// AssetAliases maps short asset names to their chain and token address
var AssetAliases = map[string]Asset{
	// Native tokens (token = "" means native)
	"eth":  {Chain: "Ethereum", Token: ""},
	"btc":  {Chain: "Bitcoin", Token: ""},
	"sol":  {Chain: "Solana", Token: ""},
	"rune": {Chain: "THORChain", Token: ""},
	"bnb":  {Chain: "BSC", Token: ""},
	"avax": {Chain: "Avalanche", Token: ""},
	"matic": {Chain: "Polygon", Token: ""},

	// Stablecoins (Ethereum mainnet)
	"usdc": {Chain: "Ethereum", Token: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"},
	"usdt": {Chain: "Ethereum", Token: "0xdAC17F958D2ee523a2206206994597C13D831ec7"},
	"dai":  {Chain: "Ethereum", Token: "0x6B175474E89094C44Da98b954EesfdDAD3Ef9ebA0"},
}

// ResolveAsset converts an asset alias to its chain and token.
// Supports formats: "eth", "usdc", "usdc:arbitrum"
func ResolveAsset(input string) Asset {
	input = strings.ToLower(input)

	// Handle "asset:chain" format (e.g., "usdc:arbitrum")
	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		assetName := parts[0]
		chainName := parts[1]

		// Get base asset
		baseAsset, ok := AssetAliases[assetName]
		if !ok {
			return Asset{Chain: "Ethereum", Token: input}
		}

		// Override chain
		baseAsset.Chain = capitalizeChain(chainName)
		return baseAsset
	}

	// Direct alias lookup
	if asset, ok := AssetAliases[input]; ok {
		return asset
	}

	// Assume it's a token address on Ethereum
	if strings.HasPrefix(input, "0x") {
		return Asset{Chain: "Ethereum", Token: input}
	}

	return Asset{Chain: "Ethereum", Token: ""}
}

// capitalizeChain converts chain name to proper case
func capitalizeChain(chain string) string {
	chainMap := map[string]string{
		"ethereum":  "Ethereum",
		"bitcoin":   "Bitcoin",
		"solana":    "Solana",
		"thorchain": "THORChain",
		"arbitrum":  "Arbitrum",
		"base":      "Base",
		"optimism":  "Optimism",
		"bsc":       "BSC",
		"avalanche": "Avalanche",
		"polygon":   "Polygon",
	}
	if proper, ok := chainMap[strings.ToLower(chain)]; ok {
		return proper
	}
	return chain
}

// GetVaultByName returns a vault by name, or the first vault if name is empty
func GetVaultByName(name string) (*LocalVault, error) {
	vaults, err := ListVaults()
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	if len(vaults) == 0 {
		return nil, fmt.Errorf("no vaults found. Import a vault first: vcli vault import")
	}

	// If no name specified, return first vault
	if name == "" {
		return vaults[0], nil
	}

	// Find by name (case-insensitive)
	nameLower := strings.ToLower(name)
	for _, v := range vaults {
		if strings.ToLower(v.Name) == nameLower {
			return v, nil
		}
	}

	// List available vaults in error message
	var names []string
	for _, v := range vaults {
		names = append(names, v.Name)
	}
	return nil, fmt.Errorf("vault '%s' not found. Available: %s", name, strings.Join(names, ", "))
}

// ConvertToSmallestUnit converts a human-readable amount to the smallest unit
func ConvertToSmallestUnit(amount string, asset Asset) string {
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return amount
	}

	decimals := 18 // Default for ETH/EVM native tokens
	if asset.Chain == "Bitcoin" {
		decimals = 8
	} else if asset.Chain == "Solana" {
		decimals = 9
	} else if asset.Token != "" {
		// ERC20 tokens - check common ones
		tokenLower := strings.ToLower(asset.Token)
		if strings.Contains(tokenLower, "a0b86991") || // USDC
			strings.Contains(tokenLower, "dac17f958") { // USDT
			decimals = 6
		}
	}

	result := f * math.Pow(10, float64(decimals))
	return fmt.Sprintf("%.0f", result)
}
