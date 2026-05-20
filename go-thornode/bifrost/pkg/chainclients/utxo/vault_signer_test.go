package utxo

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	"gitlab.com/thorchain/thornode/v3/common"
)

func TestFrostVaultSignerRemoteSignSuccess(t *testing.T) {
	expected := make([]byte, 64)
	for i := range expected {
		expected[i] = byte(i + 1)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/sign", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"scheme":"frost-secp256k1-sha256","signer":"frost-4-of-5","signature":"` + hex.EncodeToString(expected) + `"}`))
	}))
	defer server.Close()

	signer := &frostVaultSigner{
		ThorchainKeyManager: &tss.MockThorchainKeyManager{},
		endpoint:            server.URL,
		client:              server.Client(),
		log:                 zerolog.Nop(),
	}

	signature, _, err := signer.RemoteSign([]byte("payload"), common.SigningAlgoSecp256k1, "vault-pubkey")
	require.NoError(t, err)
	require.Equal(t, expected, signature)
}

func TestFrostVaultSignerRemoteSignRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "policy rejected", http.StatusForbidden)
	}))
	defer server.Close()

	signer := &frostVaultSigner{
		ThorchainKeyManager: &tss.MockThorchainKeyManager{},
		endpoint:            server.URL,
		client:              server.Client(),
		log:                 zerolog.Nop(),
	}

	_, _, err := signer.RemoteSign([]byte("payload"), common.SigningAlgoSecp256k1, "vault-pubkey")
	require.Error(t, err)
	var keysignErr tss.KeysignError
	require.ErrorAs(t, err, &keysignErr)
	require.Contains(t, keysignErr.Blame.FailReason, "FROST signer rejected request")
}

func TestFrostVaultSignerRemoteSignTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	signer := &frostVaultSigner{
		ThorchainKeyManager: &tss.MockThorchainKeyManager{},
		endpoint:            server.URL,
		client:              &http.Client{Timeout: time.Millisecond},
		log:                 zerolog.Nop(),
	}

	_, _, err := signer.RemoteSign([]byte("payload"), common.SigningAlgoSecp256k1, "vault-pubkey")
	require.Error(t, err)
	var keysignErr tss.KeysignError
	require.ErrorAs(t, err, &keysignErr)
}
