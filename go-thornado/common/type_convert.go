package common

import (
	"math/big"

	"github.com/cometbft/cometbft/crypto"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	tmsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

const (
	One       = 1e8  // One is a useful constant so THORNode doesn't need to manage 8 zeroes all the time.
	WeiPerOne = 1e18 // Relevant for chain clients which record numbers in Wei.
)

// GetSafeShare does the same as GetUncappedShare , but GetSafeShare will guarantee the result will not more than total.
// The first two arguments should always have the same units (cancelling out to represent a unitless ratio applied to the allocation).
func GetSafeShare(part, total, allocation cosmos.Uint) cosmos.Uint {
	if part.GTE(total) {
		part = total
	}
	return GetUncappedShare(part, total, allocation)
}

// GetUncappedShare calculates the share using only big.Int stack values
// and a single round-half-up at the final division for positive values.
// It avoids intermediate cosmos.Uint allocations for better performance.
func GetUncappedShare(part, total, allocation cosmos.Uint) cosmos.Uint {
	// Guard against division by zero and early zero results
	if part.IsZero() || total.IsZero() || allocation.IsZero() {
		return cosmos.ZeroUint()
	}

	// Fast equality: part == total => full allocation
	if part.Equal(total) {
		return allocation
	}

	// Copy inputs into stack big.Ints
	var a, p, t big.Int
	a.Set(allocation.BigInt())
	p.Set(part.BigInt())
	t.Set(total.BigInt())

	// num = a*p + floor(t/2)  (round-half-up for positives)
	var num big.Int
	num.Mul(&a, &p)
	var half big.Int
	half.Rsh(&t, 1) // floor(t/2)
	num.Add(&num, &half)

	// res = num / t
	var res big.Int
	res.Quo(&num, &t)

	return cosmos.NewUintFromBigInt(&res)
}

// SafeSub subtract input2 from input1, given cosmos.Uint can't be negative , otherwise it will panic
// thus in this method,when input2 is larger than input 1, it will just return cosmos.ZeroUint
func SafeSub(input1, input2 cosmos.Uint) cosmos.Uint {
	if input2.GT(input1) {
		return cosmos.ZeroUint()
	}
	return input1.Sub(input2)
}

// CosmosPrivateKeyToTMPrivateKey convert cosmos implementation of private key to tendermint private key
func CosmosPrivateKeyToTMPrivateKey(privateKey cryptotypes.PrivKey) crypto.PrivKey {
	switch k := privateKey.(type) {
	case *secp256k1.PrivKey:
		return tmsecp256k1.PrivKey(k.Bytes())
	case *ed25519.PrivKey:
		return tmed25519.PrivKey(k.Bytes())
	default:
		return nil
	}
}
