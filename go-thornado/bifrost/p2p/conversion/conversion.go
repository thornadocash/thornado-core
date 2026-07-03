package conversion

import (
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"

	crypto2 "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// GetPeerIDFromSecp256PubKey converts a secp256k1 public key into a peer.ID.
func GetPeerIDFromSecp256PubKey(pk []byte) (peer.ID, error) {
	if len(pk) == 0 {
		return "", errors.New("empty public key raw bytes")
	}
	ppk, err := crypto2.UnmarshalSecp256k1PublicKey(pk)
	if err != nil {
		return "", fmt.Errorf("fail to convert pubkey to the crypto pubkey used in libp2p: %w", err)
	}
	return peer.IDFromPublicKey(ppk)
}

func isOnCurve(x, y *big.Int, curve elliptic.Curve) bool {
	return curve.IsOnCurve(x, y)
}
