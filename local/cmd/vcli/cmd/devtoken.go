package cmd

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewDevTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "devtoken",
		Short:  "Generate a dev JWT token for local testing",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevToken()
		},
	}
}

func runDevToken() error {
	vaults, err := ListVaults()
	if err != nil || len(vaults) == 0 {
		return fmt.Errorf("no vaults found. Import a vault first")
	}
	vault := vaults[0]

	jwtSecret := []byte("devsecret")

	tokenID := uuid.New().String()
	expirationTime := time.Now().Add(7 * 24 * time.Hour)

	claims := jwt.MapClaims{
		"exp":        expirationTime.Unix(),
		"iat":        time.Now().Unix(),
		"public_key": vault.PublicKeyECDSA,
		"token_id":   tokenID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return fmt.Errorf("sign token: %w", err)
	}

	authToken := AuthToken{
		Token:     tokenString,
		PublicKey: vault.PublicKeyECDSA,
		ExpiresAt: expirationTime,
	}

	err = SaveAuthToken(&authToken)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	fmt.Println("Dev token generated successfully!")
	fmt.Printf("  Public Key: %s...\n", vault.PublicKeyECDSA[:16])
	fmt.Printf("  Expires: %s\n", expirationTime.Format(time.RFC3339))
	fmt.Printf("  Token: %s...\n", tokenString[:40])
	fmt.Println("\nNote: This token is for local dev only and won't work in production.")

	return nil
}
