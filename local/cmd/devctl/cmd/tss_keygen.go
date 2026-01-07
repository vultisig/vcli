package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/verifier/vault_config"
	vgtypes "github.com/vultisig/vultisig-go/types"
)

func (t *TSSService) KeygenWithDKLS(ctx context.Context, vaultName string) (*LocalVault, error) {
	sessionID := uuid.New().String()

	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	hexEncryptionKey := hex.EncodeToString(encryptionKey)

	chainCode := make([]byte, 32)
	_, err = rand.Read(chainCode)
	if err != nil {
		return nil, fmt.Errorf("generate chain code: %w", err)
	}
	hexChainCode := hex.EncodeToString(chainCode)

	t.logger.WithFields(logrus.Fields{
		"session_id":  sessionID,
		"local_party": t.localPartyID,
		"vault_name":  vaultName,
	}).Info("Starting DKLS keygen session")

	err = t.relayClient.RegisterSession(sessionID, t.localPartyID)
	if err != nil {
		return nil, fmt.Errorf("register session: %w", err)
	}

	t.logger.Info("Requesting Fast Vault Server to join keygen...")
	err = t.requestFastVaultKeygen(ctx, vaultName, sessionID, hexEncryptionKey, hexChainCode)
	if err != nil {
		return nil, fmt.Errorf("request fast vault keygen: %w", err)
	}

	t.logger.Info("Waiting for Fast Vault Server to join...")
	parties, err := t.waitForParties(ctx, sessionID, 2)
	if err != nil {
		return nil, fmt.Errorf("wait for parties: %w", err)
	}

	t.logger.WithField("parties", parties).Info("All parties joined")

	cfg := vault_config.Config{
		Relay: struct {
			Server string `mapstructure:"server" json:"server"`
		}{
			Server: RelayServer,
		},
		LocalPartyPrefix: t.localPartyID,
		EncryptionSecret: hexEncryptionKey[:32],
	}

	dklsService, err := vault.NewDKLSTssService(cfg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create dkls service: %w", err)
	}

	req := vgtypes.VaultCreateRequest{
		Name:             vaultName,
		SessionID:        sessionID,
		HexEncryptionKey: hexEncryptionKey,
		HexChainCode:     hexChainCode,
		LocalPartyId:     t.localPartyID,
		LibType:          1,
	}

	t.logger.Info("Running DKLS keygen protocol...")
	ecdsaPubKey, eddsaPubKey, err := dklsService.ProcessDKLSKeygen(req)
	if err != nil {
		return nil, fmt.Errorf("keygen failed: %w", err)
	}

	t.logger.WithFields(logrus.Fields{
		"ecdsa": ecdsaPubKey[:16] + "...",
		"eddsa": eddsaPubKey[:16] + "...",
	}).Info("Keygen completed successfully")

	localVault := &LocalVault{
		Name:           vaultName,
		PublicKeyECDSA: ecdsaPubKey,
		PublicKeyEdDSA: eddsaPubKey,
		HexChainCode:   hexChainCode,
		LocalPartyID:   t.localPartyID,
		Signers:        parties,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		LibType:        1,
	}

	return localVault, nil
}
