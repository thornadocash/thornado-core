package types

import (
	"github.com/mr-tron/base58"
)

const (
	PublicKeyLength = 32
	MaxSeedLength   = 32
	MaxSeed         = 16
)

type PublicKey [PublicKeyLength]byte

func (p PublicKey) String() string {
	return p.ToBase58()
}

func (p PublicKey) Bytes() []byte {
	return p[:]
}

func (p PublicKey) EqualsString(s string) bool {
	return p.ToBase58() == s
}

func MustPublicKeyFromString(s string) PublicKey {
	d, _ := base58.Decode(s)
	return PublicKeyFromBytes(d)
}

func PublicKeyFromString(s string) (PublicKey, error) {
	d, err := base58.Decode(s)
	if err != nil {
		return PublicKey{}, err
	}
	return PublicKeyFromBytes(d), nil
}

func (p PublicKey) ToBase58() string {
	return base58.Encode(p[:])
}

func PublicKeyFromBytes(b []byte) PublicKey {
	var pubkey PublicKey
	if len(b) > PublicKeyLength {
		b = b[:PublicKeyLength]
	}
	copy(pubkey[PublicKeyLength-len(b):], b)
	return pubkey
}
