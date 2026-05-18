package types

import (
	coskey "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	apicommon "gitlab.com/thorchain/thornode/v3/api/common"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// signersFromAttestations extracts signers from any type of attestation struct
// that has a PubKey field, and returns either []cosmos.AccAddress or [][]byte
func signersFromAttestations[T ~[]byte, A any](attestations []A, pubKeyGetter func(A) []byte) []T {
	signers := make([]T, len(attestations))
	for i, attestation := range attestations {
		pubKey := pubKeyGetter(attestation)
		pub := &coskey.PubKey{Key: pubKey}
		// Convert the address to the expected type
		signers[i] = T(pub.Address())
	}
	return signers
}

func commonPub(a *common.Attestation) []byte       { return a.PubKey }
func apiCommonPub(a *apicommon.Attestation) []byte { return a.PubKey }

func quorumSignersCommon(attestations []*common.Attestation) []cosmos.AccAddress {
	return signersFromAttestations[cosmos.AccAddress](attestations, commonPub)
}

func quorumSignersApiCommon(attestations []*apicommon.Attestation) [][]byte {
	return signersFromAttestations[[]byte](attestations, apiCommonPub)
}
