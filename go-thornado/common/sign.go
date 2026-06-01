package common

import (
	"encoding/base64"
	"os"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

// Sign an array of bytes.
// Returns (signature, pubkey, error)
func Sign(buf []byte) ([]byte, []byte, error) {
	kbs, err := cosmos.GetKeybase(os.Getenv(cosmos.EnvChainHome))
	if err != nil {
		return nil, nil, err
	}

	sig, err := kbs.Sign(buf)
	if err != nil {
		return nil, nil, err
	}

	pubkey, err := kbs.Keybase.Key(kbs.SignerName)
	if err != nil {
		return nil, nil, err
	}
	pk, err := pubkey.GetPubKey()
	if err != nil {
		return nil, nil, err
	}
	return sig, pk.Bytes(), nil
}

func SignBase64(buf []byte) (string, string, error) {
	sig, pubkey, err := Sign(buf)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(sig),
		base64.StdEncoding.EncodeToString(pubkey), nil
}
