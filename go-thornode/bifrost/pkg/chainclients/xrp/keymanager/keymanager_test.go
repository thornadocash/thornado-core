package keymanager

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/types"

	"github.com/stretchr/testify/require"
)

func TestSeedToXrpWallet(t *testing.T) {
	tests := []struct {
		name             string
		accountID        string
		masterPrivKeyHex string
		keyType          CryptoAlgorithm
		publicKey        string
		publicKeyHex     string
		shouldError      bool
	}{
		{
			name:             "Valid SECP256K1 seed",
			accountID:        "r4qmPsHfdoqtNMPx9popoXG3nDtsCSzUZQ",
			masterPrivKeyHex: "3869b485d5219b0582e1806595d7c6ee14fa5afed584c1b60b81e630bafcf3db",
			keyType:          SECP256K1,
			publicKey:        "aB4PwLt3AMgsvLSUWjYyun7hdGr6tcbnbAU8TKjHgHRxjXycAwS2",
			publicKeyHex:     "0237FEF6D393A2D209C879A344EFD39C20C01A8E2413298EBC6E6CCDECEEBAA7AD",
			shouldError:      false,
		},
		{
			name:             "root account SECP256K1 seed",
			accountID:        "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
			masterPrivKeyHex: "1acaaedece405b2a958212629e16f2eb46b153eee94cdd350fdeff52795525b7",
			keyType:          SECP256K1,
			publicKey:        "aBQG8RQAzjs1eTKFEAQXr2gS4utcDiEC9wmi7pfUPTi27VCahwgw",
			publicKeyHex:     "0330E7FC9D56BB25D6893BA3F317AE5BCF33B3291BD63DB32654A313222F7FD020",
			shouldError:      false,
		},
		{
			name:             "dart SECP256K1 seed",
			accountID:        "rs3xN42EFLE23gUDG2Rw4rwxhR9MnjwZKQ", // classic address
			masterPrivKeyHex: "e290da3da124b4bf9b68eb023cb57d313f016d36ef395ed03791521b83c66be6",
			// Xaddress: "X72W51px1i7iPTf4EwKFY2Nygdh5tGGNkvBFfbiuXKPxEPY"
			// XtestNetAddress: "T7Ws3yBAjFp1Fx1yWyhbSZztwhbXPqvG5a9GRHaSf1fZnqk"
			keyType:      SECP256K1,
			publicKey:    "ab4fw1tjaqpcd5eemppubbrggkax62of1nvtdbiwpxbsw7asudqn",
			publicKeyHex: "027190BF2204E1F99A9346C0717508788A73A8A3B7E5A925C349969ED1BA7FF2A0",
			shouldError:  false,
		},
		{
			name:      "dart ED25519 seed",
			accountID: "rELnd6Ae5ZYDhHkaqjSVg2vgtBnzjeDshm", // classic address
			// master seed bytes: f7f9ff93d716eaced222a3c52a3b2a36
			masterPrivKeyHex: "a53a87fb516f4f7409105e5a43a4b07ef43e42cbf7cb72b3d8020dc12f27ce14",
			// Xaddress: "XVGNvtm1P2N6A6oyQ3TWFsjyXS124KjGTNeki4i9E5DGVp1"
			// XtestNetAddress: "TVBmLzviEX8jPD22CAUH5sV1ztQ41uPJQQcDwhnCiMVzSCn"
			keyType:      ED25519,
			publicKey:    "akgguljomjqdlzfw65hf4anmcy6osaz2c3xf7ztxttcdgqtekegh",
			publicKeyHex: "EDFB7C70E528FE161ADDFDA8CB224BC19B9E6455916970F7992A356C3E77AC7EF8",
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		var privKey types.PrivKey
		switch tt.keyType {
		case ED25519:
			continue
		case SECP256K1:
			privKeyBz, _ := hex.DecodeString(tt.masterPrivKeyHex)
			privKey = &secp256k1.PrivKey{Key: privKeyBz}
		}
		t.Run(tt.name, func(t *testing.T) {
			keyManager, err := NewKeyManager(privKey)
			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, strings.ToLower(tt.accountID), strings.ToLower(keyManager.AccountID))
				require.Equal(t, tt.keyType, keyManager.KeyType)
				require.Equal(t, strings.ToLower(tt.publicKey), strings.ToLower(keyManager.PublicKey))
				require.Equal(t, strings.ToLower(tt.publicKeyHex), strings.ToLower(keyManager.PublicKeyHex))
				require.Equal(t, strings.ToLower(tt.publicKeyHex), hex.EncodeToString(keyManager.Keys.GetFormattedPublicKey()))
			}
		})
	}
}
