package secp256k1

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeriveKeysFromMasterPrivateKey_InvalidKey(t *testing.T) {
	// Empty key should fail
	keys, err := DeriveKeysFromMasterPrivateKey([]byte{})
	require.Error(t, err)
	require.Nil(t, keys)
	require.Contains(t, err.Error(), "failed to create master ECDSA key")

	// Invalid key bytes
	keys, err = DeriveKeysFromMasterPrivateKey([]byte{0x00})
	require.Error(t, err)
	require.Nil(t, keys)
}

func TestPaddedBytes_AlreadyLongEnough(t *testing.T) {
	// When bytes are exactly the right length
	val := new(big.Int).SetBytes([]byte{0x01, 0x02, 0x03, 0x04})
	result := PaddedBytes(val, 4)
	require.Len(t, result, 4)

	// When bytes are longer than requested length
	bigVal := new(big.Int).SetBytes([]byte{0x01, 0x02, 0x03, 0x04, 0x05})
	result = PaddedBytes(bigVal, 3)
	require.Len(t, result, 5) // Returns original bytes

	// When bytes need padding
	smallVal := new(big.Int).SetBytes([]byte{0x01})
	result = PaddedBytes(smallVal, 4)
	require.Len(t, result, 4)
	require.Equal(t, []byte{0x00, 0x00, 0x00, 0x01}, result)
}

func TestVerify_InvalidDERSignature(t *testing.T) {
	keys := setupTestKeys(t)
	valid, err := keys.Verify([]byte("test"), []byte("not a valid DER signature"))
	require.Error(t, err)
	require.False(t, valid)
	require.Contains(t, err.Error(), "failed to parse DER signature")
}
