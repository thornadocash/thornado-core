package ed25519

import (
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/types"
)

var Ed25519 = Ed25519Algo{}

type Ed25519Algo struct{}

func (s Ed25519Algo) Name() hd.PubKeyType {
	return hd.Ed25519Type
}

// Derive derives and returns the ed25519 private key for the given seed and HD path.
func (s Ed25519Algo) Derive() hd.DeriveFn {
	return GetPrivateKeyFromMnemonic
}

// Generate generates a ed25519 private key from the given bytes.
func (s Ed25519Algo) Generate() hd.GenerateFn {
	return func(bz []byte) types.PrivKey {
		bzArr := make([]byte, ed25519.PrivKeySize)
		copy(bzArr, bz)

		return &ed25519.PrivKey{Key: bzArr}
	}
}
