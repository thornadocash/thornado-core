package txscript

import "github.com/btcsuite/btcd/btcec"

// Signable is a interface which represent something that knows how to sign some bytes
type Signable interface {
	GetPubKey() *btcec.PublicKey
	Sign(hash []byte) (*btcec.Signature, error)
}
