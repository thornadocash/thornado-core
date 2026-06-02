package tss

import (
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/common"
)

type EncryptedKeyJSON struct {
	Address string     `json:"address"`
	Crypto  CryptoJSON `json:"crypto"`
	Id      string     `json:"id"`
	Version int        `json:"version"`
}

type CryptoJSON struct {
	Cipher       string                 `json:"cipher"`
	CipherText   string                 `json:"ciphertext"`
	CipherParams cipherparamsJSON       `json:"cipherparams"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

type cipherparamsJSON struct {
	IV string `json:"iv"`
}

// ThornadoKeyManager it is a composite of binance chain keymanager
type ThornadoKeyManager interface {
	GetPrivKey() crypto.PrivKey
	GetAddr() types.AccAddress

	ExportAsMnemonic() (string, error)
	ExportAsPrivateKey() (string, error)
	ExportAsKeyStore(password string) (*EncryptedKeyJSON, error)

	RemoteSign(msg []byte, algo common.SigningAlgo, vaultPubKey string) ([]byte, []byte, error)
	Start()
	Stop()
}
