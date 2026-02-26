package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/vultisig/vultiserver/relay"
	vgcommon "github.com/vultisig/vultisig-go/common"
	vgrelay "github.com/vultisig/vultisig-go/relay"

	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/verifier/vault_config"
)

func (t *TSSService) ReshareWithDKLS(ctx context.Context, v *LocalVault, pluginID, verifierURL, authHeader, vaultPassword string) (*LocalVault, error) {
	sessionID := uuid.New().String()

	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	hexEncryptionKey := hex.EncodeToString(encryptionKey)

	t.logger.WithFields(logrus.Fields{
		"session_id":   sessionID,
		"old_parties":  v.Signers,
		"plugin_id":    pluginID,
		"verifier_url": verifierURL,
	}).Info("Starting DKLS reshare session")

	err = t.relayClient.RegisterSession(sessionID, t.localPartyID)
	if err != nil {
		return nil, fmt.Errorf("register session: %w", err)
	}
	t.logger.WithFields(logrus.Fields{
		"session": sessionID,
		"key":     t.localPartyID,
		"body":    fmt.Sprintf("[\"%s\"]", t.localPartyID),
	}).Info("Registering session")

	t.logger.Info("Requesting Fast Vault Server to join reshare...")
	err = t.requestFastVaultReshare(ctx, v, sessionID, hexEncryptionKey, vaultPassword)
	if err != nil {
		return nil, fmt.Errorf("request fast vault reshare: %w", err)
	}

	t.logger.Info("Waiting for Fast Vault Server to join before requesting Verifier...")
	_, err = t.waitForParties(ctx, sessionID, 2)
	if err != nil {
		return nil, fmt.Errorf("fast vault did not join: %w", err)
	}
	t.logger.Info("Fast Vault Server joined successfully")

	t.logger.Info("Requesting Verifier to join reshare (with plugin)...")
	err = t.requestVerifierReshare(ctx, v, sessionID, hexEncryptionKey, pluginID, verifierURL, authHeader)
	if err != nil {
		return nil, fmt.Errorf("request verifier reshare: %w", err)
	}

	expectedParties := len(v.Signers) + 2
	t.logger.WithField("expected", expectedParties).Info("Waiting for all parties to join...")

	parties, err := t.waitForParties(ctx, sessionID, expectedParties)
	if err != nil {
		return nil, fmt.Errorf("wait for parties: %w", err)
	}

	t.logger.WithField("parties", parties).Info("All parties joined, starting reshare session")

	err = t.relayClient.StartSession(sessionID, parties)
	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

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

	t.logger.Info("Running DKLS reshare protocol (ECDSA)...")
	ecdsaPubkey, chainCode, err := t.runReshareAsInitiator(ctx, dklsService, v, sessionID, hexEncryptionKey, parties, false)
	if err != nil {
		return nil, fmt.Errorf("reshare ECDSA failed: %w", err)
	}

	t.logger.Info("Running DKLS reshare protocol (EdDSA)...")
	eddsaPubkey, _, err := t.runReshareAsInitiator(ctx, dklsService, v, sessionID, hexEncryptionKey, parties, true)
	if err != nil {
		return nil, fmt.Errorf("reshare EdDSA failed: %w", err)
	}

	err = t.relayClient.CompleteSession(sessionID, t.localPartyID)
	if err != nil {
		t.logger.WithError(err).Warn("Failed to complete session")
	}

	t.logger.WithFields(logrus.Fields{
		"ecdsa": ecdsaPubkey[:16] + "...",
		"eddsa": eddsaPubkey[:16] + "...",
	}).Info("Reshare completed successfully")

	newVault := &LocalVault{
		Name:           v.Name,
		PublicKeyECDSA: ecdsaPubkey,
		PublicKeyEdDSA: eddsaPubkey,
		HexChainCode:   chainCode,
		LocalPartyID:   v.LocalPartyID,
		Signers:        parties,
		KeyShares:      v.KeyShares,
		ResharePrefix:  sessionID[:8],
		CreatedAt:      v.CreatedAt,
		LibType:        v.LibType,
	}

	return newVault, nil
}

func (t *TSSService) runReshareAsInitiator(ctx context.Context, dklsService *vault.DKLSTssService, v *LocalVault, sessionID, hexEncryptionKey string, parties []string, isEdDSA bool) (string, string, error) {
	mpcWrapper := dklsService.GetMPCKeygenWrapper(isEdDSA)
	relayClient := vgrelay.NewRelayClient(RelayServer)

	publicKey := v.PublicKeyECDSA
	if isEdDSA {
		publicKey = v.PublicKeyEdDSA
	}

	var keyshare string
	for _, ks := range v.KeyShares {
		if ks.PubKey == publicKey {
			keyshare = ks.Keyshare
			break
		}
	}
	if keyshare == "" {
		return "", "", fmt.Errorf("keyshare not found for public key: %s", publicKey[:16])
	}

	keyshareBytes, err := base64.StdEncoding.DecodeString(keyshare)
	if err != nil {
		return "", "", fmt.Errorf("decode keyshare: %w", err)
	}

	keyshareHandle, err := mpcWrapper.KeyshareFromBytes(keyshareBytes)
	if err != nil {
		return "", "", fmt.Errorf("keyshare from bytes: %w", err)
	}
	defer func() {
		_ = mpcWrapper.KeyshareFree(keyshareHandle)
	}()

	oldPartyIndices := make([]int, 0)
	newPartyIndices := make([]int, 0)
	for i, party := range parties {
		if slices.Contains(v.Signers, party) {
			oldPartyIndices = append(oldPartyIndices, i)
		}
		newPartyIndices = append(newPartyIndices, i)
	}

	// Use same threshold formula as vultiserver: ceil(n * 2/3) - 1
	// For 4 parties: ceil(4 * 2/3) - 1 = ceil(2.67) - 1 = 3 - 1 = 2 (2-of-4)
	threshold := int(math.Ceil(float64(len(parties))*2.0/3.0)) - 1

	t.logger.WithFields(logrus.Fields{
		"parties":     parties,
		"old_indices": oldPartyIndices,
		"new_indices": newPartyIndices,
		"threshold":   threshold,
		"is_eddsa":    isEdDSA,
	}).Debug("Creating QC setup message")

	setupMsg, err := mpcWrapper.QcSetupMsgNew(keyshareHandle, threshold, parties, oldPartyIndices, newPartyIndices)
	if err != nil {
		return "", "", fmt.Errorf("create setup message: %w", err)
	}

	encodedSetupMsg := base64.StdEncoding.EncodeToString(setupMsg)
	encryptedSetupMsg, err := vgcommon.EncryptGCM(encodedSetupMsg, hexEncryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("encrypt setup message: %w", err)
	}

	messageID := ""
	if isEdDSA {
		messageID = "eddsa"
	}

	err = relayClient.UploadSetupMessage(sessionID, messageID, encryptedSetupMsg)
	if err != nil {
		return "", "", fmt.Errorf("upload setup message: %w", err)
	}

	t.logger.Debug("Setup message uploaded, creating QC session")

	sessionHandle, err := mpcWrapper.QcSessionFromSetup(setupMsg, t.localPartyID, keyshareHandle)
	if err != nil {
		return "", "", fmt.Errorf("create session from setup: %w", err)
	}

	return t.processReshareProtocol(ctx, mpcWrapper, sessionHandle, sessionID, hexEncryptionKey, parties, isEdDSA)
}

func (t *TSSService) processReshareProtocol(ctx context.Context, mpcWrapper *vault.MPCWrapperImp, sessionHandle vault.Handle, sessionID, hexEncryptionKey string, parties []string, isEdDSA bool) (string, string, error) {
	messenger := relay.NewMessenger(RelayServer, sessionID, hexEncryptionKey, true, "")
	relayClient := vgrelay.NewRelayClient(RelayServer)
	messageCache := map[string]struct{}{}

	// Drain initial outbound messages before processing inbound relay traffic.
	if err := t.sendAllReshareOutputMessages(mpcWrapper, sessionHandle, messenger, parties); err != nil {
		return "", "", fmt.Errorf("send initial reshare messages: %w", err)
	}

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		default:
		}

		if time.Since(start) > 2*time.Minute {
			return "", "", fmt.Errorf("reshare timeout")
		}

		messages, err := relayClient.DownloadMessages(sessionID, t.localPartyID, "")
		if err != nil {
			t.logger.WithError(err).Debug("Failed to download messages")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, msg := range messages {
			if msg.From == t.localPartyID {
				continue
			}

			cacheKey := fmt.Sprintf("%s-%s", sessionID, msg.Hash)
			if _, found := messageCache[cacheKey]; found {
				continue
			}

			decodedBody, err := base64.StdEncoding.DecodeString(msg.Body)
			if err != nil {
				continue
			}
			rawBody, err := vgcommon.DecryptGCM(decodedBody, hexEncryptionKey)
			if err != nil {
				continue
			}
			inboundBody, err := base64.StdEncoding.DecodeString(string(rawBody))
			if err != nil {
				continue
			}

			isFinished, err := mpcWrapper.QcSessionInputMessage(sessionHandle, inboundBody)
			if err != nil {
				t.logger.WithError(err).Debug("Failed to apply input message")
				continue
			}

			messageCache[cacheKey] = struct{}{}
			t.logger.WithFields(logrus.Fields{
				"from": msg.From,
				"hash": msg.Hash[:8],
			}).Debug("Applied message")

			_ = relayClient.DeleteMessageFromServer(sessionID, t.localPartyID, msg.Hash, "")

			if err := t.sendAllReshareOutputMessages(mpcWrapper, sessionHandle, messenger, parties); err != nil {
				t.logger.WithError(err).Debug("Failed to send reshare output messages")
				continue
			}

			if isFinished {
				t.logger.Info("Reshare protocol finished")

				result, err := mpcWrapper.QcSessionFinish(sessionHandle)
				if err != nil {
					return "", "", fmt.Errorf("finish session: %w", err)
				}

				buf, err := mpcWrapper.KeyshareToBytes(result)
				if err != nil {
					return "", "", fmt.Errorf("keyshare to bytes: %w", err)
				}

				publicKeyBytes, err := mpcWrapper.KeysharePublicKey(result)
				if err != nil {
					return "", "", fmt.Errorf("get public key: %w", err)
				}
				encodedPublicKey := hex.EncodeToString(publicKeyBytes)

				chainCode := ""
				if !isEdDSA {
					chainCodeBytes, err := mpcWrapper.KeyshareChainCode(result)
					if err != nil {
						return "", "", fmt.Errorf("get chain code: %w", err)
					}
					chainCode = hex.EncodeToString(chainCodeBytes)
				}

				encodedShare := base64.StdEncoding.EncodeToString(buf)
				t.logger.WithFields(logrus.Fields{
					"public_key": encodedPublicKey[:16] + "...",
					"share_len":  len(encodedShare),
				}).Debug("New keyshare generated")

				return encodedPublicKey, chainCode, nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (t *TSSService) sendAllReshareOutputMessages(mpcWrapper *vault.MPCWrapperImp, sessionHandle vault.Handle, messenger *relay.MessengerImp, parties []string) error {
	for {
		outbound, err := mpcWrapper.QcSessionOutputMessage(sessionHandle)
		if err != nil {
			return err
		}
		if len(outbound) == 0 {
			return nil
		}

		encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
		for i := 0; i < len(parties); i++ {
			receiver, err := mpcWrapper.QcSessionMessageReceiver(sessionHandle, outbound, i)
			if err != nil {
				t.logger.WithError(err).Debug("Failed to get receiver")
				continue
			}
			if len(receiver) == 0 {
				break
			}
			if err := messenger.Send(t.localPartyID, receiver, encodedOutbound); err != nil {
				t.logger.WithError(err).Debug("Failed to send message")
			}
		}
	}
}
