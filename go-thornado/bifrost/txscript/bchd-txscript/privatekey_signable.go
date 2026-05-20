package txscript

import "github.com/gcash/bchd/bchec"

type PrivateKeySignable struct {
	privateKey *bchec.PrivateKey
}

// NewPrivateKeySignable create a new instance of PrivateKeySignable
func NewPrivateKeySignable(priKey *bchec.PrivateKey) *PrivateKeySignable {
	return &PrivateKeySignable{privateKey: priKey}
}

// Sign the given hash bytes
func (p *PrivateKeySignable) SignECDSA(hash []byte) (*bchec.Signature, error) {
	return p.privateKey.SignECDSA(hash)
}

// Sign the given hash bytes
func (p *PrivateKeySignable) SignSchnorr(hash []byte) (*bchec.Signature, error) {
	return p.privateKey.SignSchnorr(hash)
}

// GetPubKey return the PubKey
func (p *PrivateKeySignable) GetPubKey() *bchec.PublicKey {
	return p.privateKey.PubKey()
}
