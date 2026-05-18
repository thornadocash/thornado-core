package secp256k1

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupTestKeys(t *testing.T) *Keys {
	privKey, err := hex.DecodeString("3869b485d5219b0582e1806595d7c6ee14fa5afed584c1b60b81e630bafcf3db")
	require.NoError(t, err)
	keys, err := DeriveKeysFromMasterPrivateKey(privKey)
	require.NoError(t, err)
	return keys
}

func TestGetFormattedPublicKey(t *testing.T) {
	keys := setupTestKeys(t)

	pubKey := keys.GetFormattedPublicKey()
	if len(pubKey) != 33 { // Compressed public key should be 33 bytes
		t.Errorf("Expected compressed public key length of 33 bytes, got %d", len(pubKey))
	}
	require.Equal(t, "0237fef6d393a2d209c879a344efd39c20c01a8e2413298ebc6e6ccdeceebaa7ad", hex.EncodeToString(pubKey))
}

func TestSignAndVerify(t *testing.T) {
	tests := []struct {
		name           string
		message        []byte
		expectedSigHex string
		invalidSig     bool
	}{
		{
			name:           "Valid message",
			message:        []byte("test message"),
			expectedSigHex: "3045022100f4dfe0daf188f2f1c5f298686d3ec4966f33490d2dac5cc97b0c56b6f93f334902205b3c3653470cc2a994bf97b4ec3aeed0f2534537293cca00bd904982718228ae",
			invalidSig:     false,
		},
		{
			name:           "Invalid signature",
			message:        []byte("test message"),
			expectedSigHex: "3045022101f4dfe0daf188f2f1c5f298686d3ec4966f33490d2dac5cc97b0c56b6f93f334902205b3c3653470cc2a994bf97b4ec3aeed0f2534537293cca00bd904982718228ae",
			invalidSig:     true,
		},
		{
			name:           "Empty message",
			message:        []byte{},
			expectedSigHex: "30440220790119eef5621043ad21799c9ab83bc370a11601937a89d281811ec856c4f9a30220065d061d218de149463e5aaaf7410c47f7106d3407a830960c4d681d9e65167e",
			invalidSig:     false,
		},
		{
			name:           "Long message",
			message:        bytes.Repeat([]byte("a"), 1000),
			expectedSigHex: "304502210085a880e7f6bc33270aab6499237e2a4bb578064f7c1abed1aa0e715d7acbdf5f02205ffa6ba6707d906eb93abc58d56fa6ca7bda55959688c76fa959beefa80dbe6b",
			invalidSig:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := setupTestKeys(t)

			// Test signing
			signature, err := keys.Sign(tt.message)
			require.NoError(t, err)
			if !tt.invalidSig {
				require.Equal(t, tt.expectedSigHex, hex.EncodeToString(signature))
			} else {
				signature, _ = hex.DecodeString(tt.expectedSigHex)
			}

			// Test verification
			valid, err := keys.Verify(tt.message, signature)
			if tt.invalidSig {
				require.NoError(t, err)
				require.False(t, valid)

			} else {
				require.NoError(t, err)
				require.True(t, valid)
			}
		})
	}
}

func TestSignAndVerifyModifiedMessage(t *testing.T) {
	keys := setupTestKeys(t)
	originalMessage := []byte("test message")
	modifiedMessage := []byte("test message modified")

	signature, err := keys.Sign(originalMessage)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	valid, err := keys.Verify(modifiedMessage, signature)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if valid {
		t.Error("Verify() succeeded with modified message")
	}
}
