package secp256k1

import (
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

func DeriveKeysFromMasterPrivateKey(masterPrivateKey []byte) (k *Keys, err error) {
	// Convert master private key to ECDSA format.
	masterPrivateKeyECDSA, err := crypto.ToECDSA(masterPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create master ECDSA key: %v", err)
	}

	// Get master public key from private key.
	masterPublicKey := &masterPrivateKeyECDSA.PublicKey

	// Compress the master public key.
	compressedMasterPubKey := crypto.CompressPubkey(masterPublicKey)

	return &Keys{
		masterPrivateKey:          masterPrivateKeyECDSA,
		compressedMasterPublicKey: compressedMasterPubKey,
	}, nil
}
