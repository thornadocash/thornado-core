package common

import (
	"encoding/base64"
	"os"

	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// Sign an array of bytes.
// Returns (signature, pubkey, error)
func Sign(buf []byte) ([]byte, []byte, error) {
	kbs, err := cosmos.GetKeybase(os.Getenv(cosmos.EnvChainHome))
	if err != nil {
		return nil, nil, err
	}

	// TODO: confirm this signing mode which is only for ledger devices.
	// Not applicable if ledger devices will never be used.
	// SIGN_MODE_LEGACY_AMINO_JSON will be removed in the future for SIGN_MODE_TEXTUAL
	signingMode := signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	sig, pubkey, err := kbs.Keybase.Sign(kbs.SignerName, buf, signingMode)
	if err != nil {
		return nil, nil, err
	}

	return sig, pubkey.Bytes(), nil
}

func SignBase64(buf []byte) (string, string, error) {
	sig, pubkey, err := Sign(buf)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(sig),
		base64.StdEncoding.EncodeToString(pubkey), nil
}
