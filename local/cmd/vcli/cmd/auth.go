package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthStatusCmd())
	cmd.AddCommand(newAuthLogoutCmd())

	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var vaultID string
	var password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with verifier using TSS keysign",
		Long: `Authenticate with the verifier by signing a nonce message.

This performs a TSS keysign with the Fast Vault Server to create an
EIP-191 personal_sign signature, which is then used to obtain a JWT token.

Environment variables:
  VAULT_PASSWORD  - Fast Vault password (or use -p flag)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			actualPassword := password
			if envPass := os.Getenv("VAULT_PASSWORD"); envPass != "" {
				actualPassword = envPass
			}
			return runAuthLogin(vaultID, actualPassword)
		},
	}

	cmd.Flags().StringVarP(&vaultID, "vault", "v", "", "Vault ID or public key prefix")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Fast Vault password (or set VAULT_PASSWORD)")

	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus()
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout()
		},
	}
}

type AuthToken struct {
	Token     string    `json:"token"`
	PublicKey string    `json:"public_key"`
	ExpiresAt time.Time `json:"expires_at"`
}

func runAuthLogin(vaultID, password string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	vault, err := LoadVault(vaultID)
	if err != nil {
		if vaultID == "" {
			vaults, listErr := ListVaults()
			if listErr != nil || len(vaults) == 0 {
				return fmt.Errorf("no vaults found. Import a vault first with: vcli vault import")
			}
			vault = vaults[0]
			fmt.Printf("Using vault: %s\n", vault.Name)
		} else {
			return fmt.Errorf("vault not found: %s", vaultID)
		}
	}

	if vault.PublicKeyECDSA == "" {
		return fmt.Errorf("vault has no ECDSA public key")
	}
	if vault.HexChainCode == "" {
		return fmt.Errorf("vault has no chain code")
	}

	nonceBytes := make([]byte, 16)
	_, err = rand.Read(nonceBytes)
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	expiryTime := time.Now().Add(5 * time.Minute)
	message := fmt.Sprintf("%s:%d", nonce, expiryTime.Unix())

	fmt.Printf("Authenticating with verifier...\n")
	fmt.Printf("  Vault: %s\n", vault.Name)
	fmt.Printf("  Public Key: %s...\n", vault.PublicKeyECDSA[:16])
	fmt.Printf("  Verifier: %s\n", cfg.Verifier)

	tss := NewTSSService(vault.LocalPartyID)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("\nPerforming TSS keysign for authentication...")

	derivePath := "m/44'/60'/0'/0/0"
	results, err := tss.Keysign(ctx, vault, []string{message}, derivePath, false, password)
	if err != nil {
		return fmt.Errorf("TSS keysign failed: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no signature result")
	}

	signature := results[0].DerSignature

	authReq := map[string]string{
		"message":        message,
		"signature":      signature,
		"chain_code_hex": vault.HexChainCode,
		"public_key":     vault.PublicKeyECDSA,
	}

	reqJSON, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("marshal auth request: %w", err)
	}

	url := cfg.Verifier + "/auth"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed (%d): %s", resp.StatusCode, string(body))
	}

	var authResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	err = json.Unmarshal(body, &authResp)
	if err != nil {
		return fmt.Errorf("parse auth response: %w", err)
	}

	authToken := AuthToken{
		Token:     authResp.Data.Token,
		PublicKey: vault.PublicKeyECDSA,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	err = SaveAuthToken(&authToken)
	if err != nil {
		return fmt.Errorf("save auth token: %w", err)
	}

	fmt.Println("\n✓ Authentication successful!")
	fmt.Printf("  Token expires: %s\n", authToken.ExpiresAt.Format(time.RFC3339))

	return nil
}

func runAuthStatus() error {
	token, err := LoadAuthToken()
	if err != nil {
		fmt.Println("Not authenticated.")
		fmt.Println("\nRun 'vcli auth login' to authenticate.")
		return nil
	}

	if time.Now().After(token.ExpiresAt) {
		fmt.Println("Authentication expired.")
		fmt.Println("\nRun 'vcli auth login' to re-authenticate.")
		return nil
	}

	fmt.Println("Authenticated:")
	fmt.Printf("  Public Key: %s...\n", token.PublicKey[:16])
	fmt.Printf("  Expires: %s\n", token.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("  Token: %s...\n", token.Token[:20])

	return nil
}

func runAuthLogout() error {
	err := DeleteAuthToken()
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}

	fmt.Println("Logged out successfully.")
	return nil
}

func SaveAuthToken(token *AuthToken) error {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = DefaultConfig()
	}

	cfg.AuthToken = token.Token
	cfg.AuthPublicKey = token.PublicKey
	cfg.AuthExpiresAt = token.ExpiresAt.Format(time.RFC3339)
	return SaveConfig(cfg)
}

func LoadAuthToken() (*AuthToken, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("no auth token found")
	}

	expiresAt, err := time.Parse(time.RFC3339, cfg.AuthExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expiry: %w", err)
	}

	return &AuthToken{
		Token:     cfg.AuthToken,
		PublicKey: cfg.AuthPublicKey,
		ExpiresAt: expiresAt,
	}, nil
}

func DeleteAuthToken() error {
	cfg, err := LoadConfig()
	if err != nil {
		return nil
	}

	cfg.AuthToken = ""
	cfg.AuthPublicKey = ""
	cfg.AuthExpiresAt = ""
	return SaveConfig(cfg)
}

func GetAuthHeader() (string, error) {
	token, err := LoadAuthToken()
	if err != nil {
		return "", fmt.Errorf("not authenticated. Run 'vcli auth login' first")
	}

	if time.Now().After(token.ExpiresAt) {
		return "", fmt.Errorf("authentication expired. Run 'vcli auth login' to re-authenticate")
	}

	return "Bearer " + token.Token, nil
}
