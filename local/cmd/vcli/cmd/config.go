package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
// It first checks the config, then falls back to hardcoded defaults
func GetPluginServerURL(pluginIDOrAlias string) (string, error) {
	pluginID := ResolvePluginID(pluginIDOrAlias)

	// Check config for override (environment variables or config file)
	cfg, err := LoadConfig()
	if err == nil {
		switch pluginID {
		case "vultisig-dca-0000":
			if cfg.DCAPlugin != "" && cfg.DCAPlugin != "http://localhost:8082" {
				return cfg.DCAPlugin, nil
			}
		case "vultisig-fees-feee":
			if cfg.FeePlugin != "" && cfg.FeePlugin != "http://localhost:8085" {
				return cfg.FeePlugin, nil
			}
		}
	}

	// Fall back to hardcoded defaults
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
	"eth":   {Chain: "Ethereum", Token: ""},
	"btc":   {Chain: "Bitcoin", Token: ""},
	"ltc":   {Chain: "Litecoin", Token: ""},
	"bch":   {Chain: "Bitcoin-Cash", Token: ""},
	"doge":  {Chain: "Dogecoin", Token: ""},
	"sol":   {Chain: "Solana", Token: ""},
	"rune":  {Chain: "THORChain", Token: ""},
	"bnb":   {Chain: "BSC", Token: ""},
	"avax":  {Chain: "Avalanche", Token: ""},
	"matic": {Chain: "Polygon", Token: ""},
	"zec":   {Chain: "Zcash", Token: ""},
	"dash":  {Chain: "Dash", Token: ""},
	"atom":  {Chain: "Cosmos", Token: ""},
	"cacao": {Chain: "MayaChain", Token: ""},
	"xrp":   {Chain: "Ripple", Token: ""},
	"trx":   {Chain: "Tron", Token: ""},
	"kuji":  {Chain: "Kujira", Token: ""},
	"base":  {Chain: "Base", Token: ""},

	// TRON tokens (TRC20)
	"usdt:tron": {Chain: "Tron", Token: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"},

	// Avalanche tokens
	"usdc:avalanche": {Chain: "Avalanche", Token: "0xB97EF9Ef8734C71904D8002F8B6Bc66Dd9c48a6E"},
	"usdt:avalanche": {Chain: "Avalanche", Token: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7"},

	// BSC tokens
	"usdc:bsc": {Chain: "BSC", Token: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d"},
	"usdt:bsc": {Chain: "BSC", Token: "0x55d398326f99059fF775485246999027B3197955"},
	"btcb":     {Chain: "BSC", Token: "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c"},

	// Base tokens
	"usdc:base": {Chain: "Base", Token: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"},

	// Arbitrum assets (MayaChain supported)
	"arb":           {Chain: "Arbitrum", Token: ""},
	"arb-eth":       {Chain: "Arbitrum", Token: ""},
	"arb-usdc":      {Chain: "Arbitrum", Token: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831"},
	"usdc:arbitrum": {Chain: "Arbitrum", Token: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831"},
	"arb-usdt":      {Chain: "Arbitrum", Token: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9"},
	"usdt:arbitrum": {Chain: "Arbitrum", Token: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9"},
	"arb-wbtc":      {Chain: "Arbitrum", Token: "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f"},
	"wbtc:arbitrum": {Chain: "Arbitrum", Token: "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f"},

	// Stablecoins (Ethereum mainnet)
	"usdc": {Chain: "Ethereum", Token: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"},
	"usdt": {Chain: "Ethereum", Token: "0xdAC17F958D2ee523a2206206994597C13D831ec7"},
	"dai":  {Chain: "Ethereum", Token: "0x6B175474E89094C44Da98b954EesfdDAD3Ef9ebA0"},

	// ERC20 tokens (Ethereum mainnet)
	"wbtc": {Chain: "Ethereum", Token: "0x2260FAC5E5542A773Aa44fBCfeDf7C193bc2C599"},
	"link": {Chain: "Ethereum", Token: "0x514910771AF9Ca656af840dff83E8264EcF986CA"},
}

// ResolveAsset converts an asset alias to its chain and token.
// Supports formats: "eth", "usdc", "usdc:arbitrum", "usdt:tron", "BASE.ETH", "BSC.USDT"
func ResolveAsset(input string) Asset {
	input = strings.ToLower(input)

	// First check for exact match (handles "usdt:tron" style aliases)
	if asset, ok := AssetAliases[input]; ok {
		return asset
	}

	// Handle THORChain "CHAIN.TOKEN" format (e.g., "base.eth", "bsc.usdt", "base.usdc-0x833...")
	if strings.Contains(input, ".") {
		parts := strings.SplitN(input, ".", 2)
		chainName := parts[0]
		tokenPart := parts[1]

		chain := capitalizeChain(chainName)

		// Handle native token (e.g., "base.eth", "bsc.bnb", "avax.avax")
		if isNativeToken(chainName, tokenPart) {
			return Asset{Chain: chain, Token: ""}
		}

		// Handle token with address (e.g., "base.usdc-0x833589...")
		if strings.Contains(tokenPart, "-0x") {
			addrParts := strings.SplitN(tokenPart, "-", 2)
			tokenAddr := addrParts[1]
			return Asset{Chain: chain, Token: tokenAddr}
		}

		// Handle token symbol (e.g., "base.usdc", "bsc.usdt")
		tokenAddr := resolveTokenOnChain(chain, tokenPart)
		return Asset{Chain: chain, Token: tokenAddr}
	}

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

	// Assume it's a token address on Ethereum
	if strings.HasPrefix(input, "0x") {
		return Asset{Chain: "Ethereum", Token: input}
	}

	return Asset{Chain: "Ethereum", Token: ""}
}

// isNativeToken checks if the token is the native token for the chain
func isNativeToken(chain, token string) bool {
	nativeTokens := map[string][]string{
		"base":      {"eth"},
		"arbitrum":  {"eth"},
		"arb":       {"eth"},
		"optimism":  {"eth"},
		"op":        {"eth"},
		"ethereum":  {"eth"},
		"bsc":       {"bnb"},
		"bnb":       {"bnb"},
		"avalanche": {"avax"},
		"avax":      {"avax"},
		"polygon":   {"matic"},
		"matic":     {"matic"},
	}
	tokens, ok := nativeTokens[chain]
	if !ok {
		return false
	}
	for _, t := range tokens {
		if t == token {
			return true
		}
	}
	return false
}

// resolveTokenOnChain resolves a token symbol to its address on a specific chain
func resolveTokenOnChain(chain, token string) string {
	// Known token addresses per chain (THORChain supported tokens)
	tokenAddresses := map[string]map[string]string{
		"Base": {
			"usdc":  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
			"cbbtc": "0xcbB7C0000AB88B473b1f5aFd9ef808440eed33Bf",
		},
		"BSC": {
			"usdc": "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d",
			"usdt": "0x55d398326f99059fF775485246999027B3197955",
			"btcb": "0x7130d2A12B9BCbFAe4f2634d864A1Ee1Ce3Ead9c",
		},
		"Avalanche": {
			"usdc": "0xB97EF9Ef8734C71904D8002F8B6Bc66Dd9c48a6E",
			"usdt": "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7",
		},
		"Arbitrum": {
			"usdc": "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
			"usdt": "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9",
			"wbtc": "0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f",
		},
		"Ethereum": {
			"usdc": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			"usdt": "0xdAC17F958D2ee523a2206206994597C13D831ec7",
			"wbtc": "0x2260FAC5E5542A773Aa44fBCfeDf7C193bc2C599",
		},
	}

	if chainTokens, ok := tokenAddresses[chain]; ok {
		if addr, ok := chainTokens[token]; ok {
			return addr
		}
	}
	return ""
}

// capitalizeChain converts chain name to proper case
func capitalizeChain(chain string) string {
	chainMap := map[string]string{
		"ethereum":  "Ethereum",
		"bitcoin":   "Bitcoin",
		"solana":    "Solana",
		"thorchain": "THORChain",
		"arbitrum":  "Arbitrum",
		"arb":       "Arbitrum",
		"base":      "Base",
		"optimism":  "Optimism",
		"op":        "Optimism",
		"bsc":       "BSC",
		"bnb":       "BSC",
		"avalanche": "Avalanche",
		"avax":      "Avalanche",
		"polygon":   "Polygon",
		"matic":     "Polygon",
		"tron":      "Tron",
		"trx":       "Tron",
		"ripple":    "Ripple",
		"xrp":       "Ripple",
		"mayachain": "MayaChain",
		"maya":      "MayaChain",
		"dash":      "Dash",
		"zcash":     "Zcash",
		"zec":       "Zcash",
		"kujira":    "Kujira",
		"kuji":      "Kujira",
	}
	if proper, ok := chainMap[strings.ToLower(chain)]; ok {
		return proper
	}
	return chain
}

// chainRPCEnvVars maps chain names to their RPC environment variable names
var chainRPCEnvVars = map[string]string{
	"Ethereum":  "RPC_ETHEREUM_URL",
	"Arbitrum":  "RPC_ARBITRUM_URL",
	"Base":      "RPC_BASE_URL",
	"Optimism":  "RPC_OPTIMISM_URL",
	"Polygon":   "RPC_POLYGON_URL",
	"BSC":       "RPC_BSC_URL",
	"Avalanche": "RPC_AVALANCHE_URL",
	"Blast":     "RPC_BLAST_URL",
	"ZkSync":    "RPC_ZKSYNC_URL",
	"Cronos":    "RPC_CRONOS_URL",
}

// defaultRPCURLs provides fallback RPC URLs when env vars are not set
var defaultRPCURLs = map[string]string{
	"Ethereum":  "https://ethereum-rpc.publicnode.com",
	"Arbitrum":  "https://arbitrum-one-rpc.publicnode.com",
	"Base":      "https://base-rpc.publicnode.com",
	"Optimism":  "https://optimism-rpc.publicnode.com",
	"Polygon":   "https://polygon-bor-rpc.publicnode.com",
	"BSC":       "https://bsc-rpc.publicnode.com",
	"Avalanche": "https://avalanche-c-chain-rpc.publicnode.com",
}

// getChainRPCURL returns the RPC URL for a chain, checking env vars first
func getChainRPCURL(chain string) (string, bool) {
	if envVar, ok := chainRPCEnvVars[chain]; ok {
		if url := os.Getenv(envVar); url != "" {
			return url, true
		}
	}
	if url, ok := defaultRPCURLs[chain]; ok {
		return url, true
	}
	return "", false
}

// queryERC20Decimals queries the decimals() function of an ERC20 token contract
func queryERC20Decimals(chain, tokenAddress string) (int, error) {
	rpcURL, ok := getChainRPCURL(chain)
	if !ok {
		return 0, fmt.Errorf("no RPC URL for chain: %s", chain)
	}

	// decimals() function selector: 0x313ce567
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []any{
			map[string]string{
				"to":   tokenAddress,
				"data": "0x313ce567",
			},
			"latest",
		},
		"id": 1,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return 0, fmt.Errorf("RPC error: %s", result.Error.Message)
	}

	if result.Result == "" || result.Result == "0x" {
		return 0, fmt.Errorf("empty result from decimals()")
	}

	// Parse hex result (32 bytes, but decimals is just a uint8)
	hexStr := strings.TrimPrefix(result.Result, "0x")
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return 0, fmt.Errorf("decode hex: %w", err)
	}

	if len(decoded) == 0 {
		return 0, fmt.Errorf("empty decoded result")
	}

	// Decimals is the last byte (uint8)
	decimals := int(decoded[len(decoded)-1])
	return decimals, nil
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
// Uses big.Int arithmetic to avoid floating point precision errors
func ConvertToSmallestUnit(amount string, asset Asset) string {
	decimals := getChainDecimals(asset)

	// Split into integer and fractional parts
	parts := strings.Split(amount, ".")
	intPart := parts[0]
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	// Pad or truncate fractional part to match decimals
	if len(fracPart) < decimals {
		fracPart = fracPart + strings.Repeat("0", decimals-len(fracPart))
	} else if len(fracPart) > decimals {
		fracPart = fracPart[:decimals]
	}

	// Combine integer and fractional parts
	combined := intPart + fracPart

	// Remove leading zeros but keep at least one digit
	combined = strings.TrimLeft(combined, "0")
	if combined == "" {
		combined = "0"
	}

	// Validate it's a valid number
	result := new(big.Int)
	_, ok := result.SetString(combined, 10)
	if !ok {
		return amount
	}

	return result.String()
}

func getChainDecimals(asset Asset) int {
	// For tokens, determine decimals based on chain and token
	if asset.Token != "" {
		// Handle TRON TRC-20 tokens first (not EVM compatible)
		if asset.Chain == "Tron" {
			// TRON USDT uses 6 decimals
			if strings.EqualFold(asset.Token, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t") {
				return 6
			}
			return 6 // Most TRC-20 tokens use 6 decimals
		}

		// For ERC20 tokens, query the contract for decimals
		decimals, err := queryERC20Decimals(asset.Chain, asset.Token)
		if err == nil {
			return decimals
		}
		// Fall back to known tokens if query fails
		tokenLower := strings.ToLower(asset.Token)
		if strings.Contains(tokenLower, "a0b86991") || // USDC
			strings.Contains(tokenLower, "dac17f958") { // USDT
			return 6
		}
		if strings.Contains(tokenLower, "2260fac5") { // WBTC
			return 8
		}
		return 18 // Default for ERC20
	}

	// Native token decimals by chain
	switch asset.Chain {
	case "Bitcoin", "Litecoin", "Bitcoin-Cash", "Dogecoin", "Zcash", "Dash", "THORChain":
		return 8
	case "Cosmos", "Osmosis", "Dydx", "Kujira", "MayaChain", "Ripple", "Tron":
		return 6
	case "Solana":
		return 9
	default:
		return 18 // ETH/EVM chains
	}
}
