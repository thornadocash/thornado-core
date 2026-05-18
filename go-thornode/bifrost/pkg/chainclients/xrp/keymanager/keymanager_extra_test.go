package keymanager

import (
	"encoding/hex"
	"testing"

	cosmoscryptoed25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cosmoscryptosecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"
)

func getTestKeyManager(t *testing.T) *KeyManager {
	t.Helper()
	privKeyBz, err := hex.DecodeString("3869b485d5219b0582e1806595d7c6ee14fa5afed584c1b60b81e630bafcf3db")
	require.NoError(t, err)
	privKey := &cosmoscryptosecp256k1.PrivKey{Key: privKeyBz}
	km, err := NewKeyManager(privKey)
	require.NoError(t, err)
	return km
}

func TestNewKeyManager_Ed25519Error(t *testing.T) {
	privKey := cosmoscryptoed25519.GenPrivKey()
	km, err := NewKeyManager(privKey)
	require.Error(t, err)
	require.Nil(t, km)
	require.Contains(t, err.Error(), "ed25519 key is not supported")
}

func TestNewKeyManager_UnsupportedKeyType(t *testing.T) {
	// Use a nil key to trigger the default case
	km, err := NewKeyManager(nil)
	require.Error(t, err)
	require.Nil(t, km)
	require.Contains(t, err.Error(), "unsupported key type")
}

func TestKeyManager_Pubkey(t *testing.T) {
	km := getTestKeyManager(t)
	pubkey := km.Pubkey()
	require.NotEmpty(t, pubkey)
	require.Equal(t, km.PublicKey, pubkey)
}

func TestKeyManager_GetAddr(t *testing.T) {
	km := getTestKeyManager(t)
	addr := km.GetAddr()
	require.NotEmpty(t, addr)
	require.Equal(t, km.AccountID, addr)
}

func TestKeyManager_Sign(t *testing.T) {
	km := getTestKeyManager(t)
	sig, err := km.Sign([]byte("test message"))
	require.NoError(t, err)
	require.NotEmpty(t, sig)
}

func TestCryptoAlgorithm_String(t *testing.T) {
	require.Equal(t, "SECP256K1", SECP256K1.String())
	require.Equal(t, "ED25519", ED25519.String())
	// Out of range value
	require.Contains(t, CryptoAlgorithm(99).String(), "CryptoAlgorithm(99)")
}

func TestEncode(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	prefix := []byte{0x00}
	encoded := Encode(data, prefix)
	require.NotEmpty(t, encoded)
}

func TestChecksum(t *testing.T) {
	input := []byte("test data")
	cksum := checksum(input)
	require.Len(t, cksum, 4)
	// Same input should produce same checksum
	cksum2 := checksum(input)
	require.Equal(t, cksum, cksum2)
	// Different input should produce different checksum
	cksum3 := checksum([]byte("different"))
	require.NotEqual(t, cksum, cksum3)
}
