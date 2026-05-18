package tss

import (
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common"
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

// ThorchainKeyManager it is a composite of binance chain keymanager
type ThorchainKeyManager interface {
	GetPrivKey() crypto.PrivKey
	GetAddr() types.AccAddress

	ExportAsMnemonic() (string, error)
	ExportAsPrivateKey() (string, error)
	ExportAsKeyStore(password string) (*EncryptedKeyJSON, error)

	RemoteSign(msg []byte, algo common.SigningAlgo, poolPubKey string) ([]byte, []byte, error)
	Start()
	Stop()
}
