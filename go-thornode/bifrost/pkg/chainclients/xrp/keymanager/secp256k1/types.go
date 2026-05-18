package secp256k1

import (
	"crypto/ecdsa"
	"math/big"
)

type Keys struct {
	masterPrivateKey          *ecdsa.PrivateKey
	compressedMasterPublicKey []byte
}

// ECDSASignature represents the R and S components of a signature.
type ECDSASignature struct {
	R, S *big.Int
}
