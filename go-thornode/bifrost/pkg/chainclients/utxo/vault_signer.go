package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cometbft/cometbft/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"

	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	gotss "gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/tss"
	"gitlab.com/thorchain/thornode/v3/common"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

const (
	frostSignerEndpointEnv = "FROST_SIGNER_ENDPOINT"
	frostSignerTimeoutEnv  = "FROST_SIGNER_TIMEOUT"
	defaultFrostTimeout    = 30 * time.Second
)

type frostVaultSigner struct {
	tss.ThorchainKeyManager
	endpoint string
	client   *http.Client
	log      zerolog.Logger
}

type frostSignRequest struct {
	Withdrawal frostWithdrawalRequest `json:"withdrawal"`
}

type frostWithdrawalRequest struct {
	WithdrawalID  string `json:"withdrawal_id"`
	Recipient     string `json:"recipient"`
	AmountSats    uint64 `json:"amount_sats"`
	FeeSats       uint64 `json:"fee_sats"`
	NullifierHash string `json:"nullifier_hash"`
}

type frostSignResponse struct {
	Scheme         string `json:"scheme"`
	Signer         string `json:"signer"`
	MessageDigest  string `json:"message_digest"`
	GroupPublicKey string `json:"group_public_key"`
	Signature      string `json:"signature"`
}

func newVaultSigner(server *gotss.TssServer, bridge thorclient.ThorchainBridge, log zerolog.Logger) (tss.ThorchainKeyManager, error) {
	keysign, err := tss.NewKeySign(server, bridge)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(os.Getenv(frostSignerEndpointEnv), "/")
	if endpoint == "" {
		return keysign, nil
	}

	return &frostVaultSigner{
		ThorchainKeyManager: keysign,
		endpoint:            endpoint,
		client: &http.Client{
			Timeout: frostSignerTimeout(),
		},
		log: log,
	}, nil
}

func frostSignerTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(frostSignerTimeoutEnv))
	if raw == "" {
		return defaultFrostTimeout
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return defaultFrostTimeout
	}
	return timeout
}

func (s *frostVaultSigner) RemoteSign(msg []byte, algo common.SigningAlgo, poolPubKey string) ([]byte, []byte, error) {
	if algo != common.SigningAlgoSecp256k1 {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("FROST signer only supports secp256k1, got %s", algo),
		})
	}

	payload, err := json.Marshal(frostSignRequest{
		Withdrawal: frostWithdrawalRequest{
			WithdrawalID:  frostSessionID(poolPubKey, msg),
			Recipient:     "thornode-bifrost-raw-payload",
			AmountSats:    0,
			FeeSats:       0,
			NullifierHash: hex.EncodeToString(msg),
		},
	})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodPost, s.endpoint+"/v1/sign", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("FROST signer rejected request: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))),
		})
	}

	var result frostSignResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil, err
	}
	signature, err := hex.DecodeString(result.Signature)
	if err != nil {
		return nil, nil, fmt.Errorf("decode FROST signature: %w", err)
	}
	if len(signature) != 64 {
		return nil, nil, fmt.Errorf("FROST signature must be 64 bytes, got %d", len(signature))
	}

	s.log.Debug().
		Str("scheme", result.Scheme).
		Str("signer", result.Signer).
		Str("pool_pub_key", poolPubKey).
		Msg("received FROST vault signature")
	return signature, nil, nil
}

func frostSessionID(poolPubKey string, msg []byte) string {
	sum := sha256.Sum256(append([]byte(poolPubKey), msg...))
	return hex.EncodeToString(sum[:])
}

func (s *frostVaultSigner) GetPrivKey() crypto.PrivKey {
	return s.ThorchainKeyManager.GetPrivKey()
}

func (s *frostVaultSigner) GetAddr() sdk.AccAddress {
	return s.ThorchainKeyManager.GetAddr()
}

func (s *frostVaultSigner) ExportAsMnemonic() (string, error) {
	return s.ThorchainKeyManager.ExportAsMnemonic()
}

func (s *frostVaultSigner) ExportAsPrivateKey() (string, error) {
	return s.ThorchainKeyManager.ExportAsPrivateKey()
}

func (s *frostVaultSigner) ExportAsKeyStore(password string) (*tss.EncryptedKeyJSON, error) {
	return s.ThorchainKeyManager.ExportAsKeyStore(password)
}

func (s *frostVaultSigner) Start() {
	s.ThorchainKeyManager.Start()
}

func (s *frostVaultSigner) Stop() {
	s.ThorchainKeyManager.Stop()
}
