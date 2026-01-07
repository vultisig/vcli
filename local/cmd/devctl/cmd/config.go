package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func DefaultConfig() *DevConfig {
	return &DevConfig{
		Verifier:    "http://localhost:8080",
		FeePlugin:   "http://localhost:8085",
		DCAPlugin:   "http://localhost:8082",
		RelayServer: "https://api.vultisig.com/router",
		DatabaseDSN: "postgres://vultisig:vultisig@localhost:5432/vultisig-verifier?sslmode=disable",
		RedisURI:    "redis://:vultisig@localhost:6379",
		MinioHost:   "http://localhost:9000",
		MinioAccess: "minioadmin",
		MinioSecret: "minioadmin",
		Encryption:  "dev-encryption-secret-32b",
	}
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vultisig", "devctl.json")
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
