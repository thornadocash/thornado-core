package txscript

import "github.com/gcash/bchd/bchec"

// Signable is a interface which represent something that knows how to sign some bytes
type Signable interface {
	GetPubKey() *bchec.PublicKey
	SignECDSA(hash []byte) (*bchec.Signature, error)
	SignSchnorr(hash []byte) (*bchec.Signature, error)
}
