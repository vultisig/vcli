package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	vgcommon "github.com/vultisig/vultisig-go/common"
	vgrelay "github.com/vultisig/vultisig-go/relay"
	"github.com/vultisig/vultiserver/relay"

	"github.com/vultisig/verifier/vault"
	"github.com/vultisig/verifier/vault_config"
)

func (t *TSSService) KeysignWithDKLS(ctx context.Context, v *LocalVault, messages []string, derivePath, verifierURL, pluginID, authHeader string) ([]KeysignResult, error) {
	return t.KeysignWithFastVault(ctx, v, messages, derivePath, "")
}

func (t *TSSService) KeysignWithFastVault(ctx context.Context, v *LocalVault, messages []string, derivePath, vaultPassword string) ([]KeysignResult, error) {
	sessionID := uuid.New().String()

	encryptionKey := make([]byte, 32)
	_, err := rand.Read(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	hexEncryptionKey := hex.EncodeToString(encryptionKey)

	t.logger.WithFields(logrus.Fields{
		"session_id":  sessionID,
		"public_key":  v.PublicKeyECDSA[:16] + "...",
		"messages":    len(messages),
		"derive_path": derivePath,
	}).Info("Starting DKLS keysign with Fast Vault Server")

	err = t.relayClient.RegisterSession(sessionID, t.localPartyID)
	if err != nil {
		return nil, fmt.Errorf("register session: %w", err)
	}

	t.logger.Info("Requesting Fast Vault Server to join keysign...")
	err = t.requestFastVaultKeysignDKLS(ctx, v, sessionID, hexEncryptionKey, messages, derivePath, vaultPassword)
	if err != nil {
		return nil, fmt.Errorf("request fast vault keysign: %w", err)
	}

	t.logger.Info("Waiting for Fast Vault Server to join...")
	parties, err := t.waitForParties(ctx, sessionID, 2)
	if err != nil {
		return nil, fmt.Errorf("wait for parties: %w", err)
	}

	t.logger.WithField("parties", parties).Info("All parties joined, starting keysign")

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

	mpcWrapper := dklsService.GetMPCKeygenWrapper(false)

	results := make([]KeysignResult, len(messages))
	for i, msg := range messages {
		t.logger.WithField("message_index", i).Info("Running DKLS keysign protocol...")

		result, err := t.runKeysignAsInitiator(mpcWrapper, v, sessionID, hexEncryptionKey, parties, msg, derivePath, i)
		if err != nil {
			return nil, fmt.Errorf("keysign message %d failed: %w", i, err)
		}
		results[i] = *result
	}

	err = t.relayClient.CompleteSession(sessionID, t.localPartyID)
	if err != nil {
		t.logger.WithError(err).Warn("Failed to complete session")
	}

	t.logger.WithField("signatures", len(results)).Info("Keysign completed successfully")
	return results, nil
}

func (t *TSSService) requestFastVaultKeysignDKLS(ctx context.Context, v *LocalVault, sessionID, hexEncKey string, messages []string, derivePath, vaultPassword string) error {
	type FastVaultSignRequest struct {
		PublicKey        string   `json:"public_key"`
		Messages         []string `json:"messages"`
		Session          string   `json:"session"`
		HexEncryptionKey string   `json:"hex_encryption_key"`
		DerivePath       string   `json:"derive_path"`
		IsECDSA          bool     `json:"is_ecdsa"`
		VaultPassword    string   `json:"vault_password"`
	}

	req := FastVaultSignRequest{
		PublicKey:        v.PublicKeyECDSA,
		Messages:         messages,
		Session:          sessionID,
		HexEncryptionKey: hexEncKey,
		DerivePath:       derivePath,
		IsECDSA:          true,
		VaultPassword:    vaultPassword,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	t.logger.WithField("request", string(reqJSON)).Debug("Sending keysign request to Fast Vault Server")

	url := FastVaultServer + "/vault/sign"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fast vault server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (t *TSSService) runKeysignAsInitiator(mpcWrapper *vault.MPCWrapperImp, v *LocalVault, sessionID, hexEncryptionKey string, parties []string, message, derivePath string, msgIndex int) (*KeysignResult, error) {
	relayClient := vgrelay.NewRelayClient(RelayServer)

	publicKey := v.PublicKeyECDSA

	var keyshare string
	for _, ks := range v.KeyShares {
		if ks.PubKey == publicKey {
			keyshare = ks.Keyshare
			break
		}
	}
	if keyshare == "" {
		return nil, fmt.Errorf("keyshare not found for public key: %s", publicKey[:16])
	}

	keyshareBytes, err := base64.StdEncoding.DecodeString(keyshare)
	if err != nil {
		return nil, fmt.Errorf("decode keyshare: %w", err)
	}

	keyshareHandle, err := mpcWrapper.KeyshareFromBytes(keyshareBytes)
	if err != nil {
		return nil, fmt.Errorf("keyshare from bytes: %w", err)
	}
	defer func() {
		_ = mpcWrapper.KeyshareFree(keyshareHandle)
	}()

	keyID, err := mpcWrapper.KeyshareKeyID(keyshareHandle)
	if err != nil {
		return nil, fmt.Errorf("get key id: %w", err)
	}

	md5Hash := md5.Sum([]byte(message))
	messageID := hex.EncodeToString(md5Hash[:])

	messageBytes, err := hex.DecodeString(message)
	if err != nil {
		return nil, fmt.Errorf("message must be hex-encoded 32-byte hash: %w", err)
	}
	if len(messageBytes) != 32 {
		return nil, fmt.Errorf("message must be 32 bytes, got %d", len(messageBytes))
	}

	derivePathBytes := fmtDerivePath(derivePath)
	idsBytes := fmtIdsSlice(parties)

	t.logger.WithFields(logrus.Fields{
		"parties":     parties,
		"derive_path": derivePath,
		"msg_len":     len(messageBytes),
		"message_id":  messageID,
	}).Debug("Creating keysign setup message")

	setupMsg, err := mpcWrapper.SignSetupMsgNew(keyID, derivePathBytes, messageBytes, idsBytes)
	if err != nil {
		return nil, fmt.Errorf("create setup message: %w", err)
	}

	encodedSetupMsg := base64.StdEncoding.EncodeToString(setupMsg)
	encryptedSetupMsg, err := vgcommon.EncryptGCM(encodedSetupMsg, hexEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt setup message: %w", err)
	}

	err = relayClient.UploadSetupMessage(sessionID, messageID, encryptedSetupMsg)
	if err != nil {
		return nil, fmt.Errorf("upload setup message: %w", err)
	}

	t.logger.Debug("Setup message uploaded, creating keysign session")

	sessionHandle, err := mpcWrapper.SignSessionFromSetup(setupMsg, []byte(t.localPartyID), keyshareHandle)
	if err != nil {
		return nil, fmt.Errorf("create session from setup: %w", err)
	}

	return t.processKeysignProtocol(mpcWrapper, sessionHandle, sessionID, hexEncryptionKey, parties, messageID)
}

func fmtDerivePath(path string) []byte {
	if path == "" {
		return nil
	}
	return []byte(strings.ReplaceAll(path, "'", ""))
}

func fmtIdsSlice(ids []string) []byte {
	return []byte(strings.Join(ids, "\x00"))
}

func (t *TSSService) processKeysignProtocol(mpcWrapper *vault.MPCWrapperImp, sessionHandle vault.Handle, sessionID, hexEncryptionKey string, parties []string, messageID string) (*KeysignResult, error) {
	messenger := relay.NewMessenger(RelayServer, sessionID, hexEncryptionKey, true, messageID)
	relayClient := vgrelay.NewRelayClient(RelayServer)
	var messageCache sync.Map

	go func() {
		for {
			outbound, err := mpcWrapper.SignSessionOutputMessage(sessionHandle)
			if err != nil {
				t.logger.WithError(err).Debug("Failed to get output message")
				return
			}
			if len(outbound) == 0 {
				return
			}

			encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
			for i := 0; i < len(parties); i++ {
				receiver, err := mpcWrapper.SignSessionMessageReceiver(sessionHandle, outbound, i)
				if err != nil {
					t.logger.WithError(err).Debug("Failed to get receiver")
					continue
				}
				if len(receiver) == 0 {
					break
				}

				t.logger.WithField("receiver", string(receiver)).Debug("Sending message")
				_ = messenger.Send(t.localPartyID, string(receiver), encodedOutbound)
			}
		}
	}()

	start := time.Now()
	for {
		if time.Since(start) > 2*time.Minute {
			return nil, fmt.Errorf("keysign timeout")
		}

		messages, err := relayClient.DownloadMessages(sessionID, t.localPartyID, messageID)
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
			if _, found := messageCache.Load(cacheKey); found {
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

			isFinished, err := mpcWrapper.SignSessionInputMessage(sessionHandle, inboundBody)
			if err != nil {
				t.logger.WithError(err).Debug("Failed to apply input message")
				continue
			}

			messageCache.Store(cacheKey, true)
			t.logger.WithFields(logrus.Fields{
				"from": msg.From,
				"hash": msg.Hash[:8],
			}).Debug("Applied message")

			_ = relayClient.DeleteMessageFromServer(sessionID, t.localPartyID, msg.Hash, messageID)

			for {
				outbound, err := mpcWrapper.SignSessionOutputMessage(sessionHandle)
				if err != nil || len(outbound) == 0 {
					break
				}
				encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
				for i := 0; i < len(parties); i++ {
					receiver, _ := mpcWrapper.SignSessionMessageReceiver(sessionHandle, outbound, i)
					if len(receiver) == 0 {
						break
					}
					_ = messenger.Send(t.localPartyID, string(receiver), encodedOutbound)
				}
			}

			if isFinished {
				t.logger.Info("Keysign protocol finished")

				signature, err := mpcWrapper.SignSessionFinish(sessionHandle)
				if err != nil {
					return nil, fmt.Errorf("finish session: %w", err)
				}

				r := hex.EncodeToString(signature[:32])
				s := hex.EncodeToString(signature[32:64])
				recoveryID := "1b"
				if len(signature) > 64 {
					recoveryID = fmt.Sprintf("%02x", signature[64])
				}

				t.logger.WithFields(logrus.Fields{
					"r": r[:16] + "...",
					"s": s[:16] + "...",
				}).Debug("Signature generated")

				return &KeysignResult{
					R:            r,
					S:            s,
					RecoveryID:   recoveryID,
					DerSignature: hex.EncodeToString(signature),
				}, nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}
