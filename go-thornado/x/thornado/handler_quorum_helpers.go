package thornado

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// deduplicateAttestations returns attestations deduplicated by pubkey, capped at maxCount.
// This prevents a malicious block proposer from including thousands of duplicate attestations
// to cause excessive signature verification and KV store operations.
func deduplicateAttestations(attestations []*common.Attestation, maxCount int) []*common.Attestation {
	if len(attestations) == 0 {
		return attestations
	}
	seen := make(map[string]struct{})
	result := make([]*common.Attestation, 0, min(len(attestations), maxCount))
	for _, att := range attestations {
		if att == nil {
			continue
		}
		key := string(att.PubKey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, att)
		if len(result) >= maxCount {
			break
		}
	}
	return result
}

func verifyQuorumAttestation(activeNodeAccounts NodeAccounts, signBz []byte, att *common.Attestation) (cosmos.AccAddress, error) {
	if att == nil {
		return nil, fmt.Errorf("attestation is nil")
	}
	if len(att.PubKey) == 0 {
		return nil, fmt.Errorf("pubkey is empty")
	}
	if len(att.Signature) == 0 {
		return nil, fmt.Errorf("signature is empty")
	}

	pk := secp256k1.PubKey{Key: att.PubKey}

	bech32Pub, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, &pk)
	if err != nil {
		return nil, fmt.Errorf("fail to get bech32 pub key: %w", err)
	}

	// check if the signer is an active node account
	found := false
	for _, node := range activeNodeAccounts {
		if bech32Pub == node.PubKeySet.Secp256k1.String() {
			found = true
			break
		}
	}
	if !found {
		// can occur if a node account churns out before the tx is processed.
		return nil, fmt.Errorf("signer is not an active node account: %s", pk.String())
	}

	if !pk.VerifySignature(signBz, att.Signature) {
		return nil, fmt.Errorf("failed to verify signature: %s", pk.String())
	}

	return cosmos.AccAddress(pk.Address()), nil
}
