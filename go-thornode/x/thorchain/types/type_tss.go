package types

import (
	"sort"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// NewTssVoter create a new instance of TssVoter
func NewTssVoter(id string, pks []string, pool, poolPubKeyEddsa common.PubKey) TssVoter {
	return TssVoter{
		ID:              id,
		PubKeys:         pks,
		PoolPubKey:      pool,
		PoolPubKeyEddsa: poolPubKeyEddsa,
	}
}

func (m *TssVoter) GetChains() common.Chains {
	chains := make(common.Chains, 0)
	for _, c := range m.Chains {
		chain, err := common.NewChain(c)
		if err != nil {
			continue
		}
		chains = append(chains, chain)
	}
	return chains
}

func (m *TssVoter) GetPubKeys() common.PubKeys {
	pubkeys := make(common.PubKeys, 0)
	for _, pk := range m.PubKeys {
		pk, err := common.NewPubKey(pk)
		if err != nil {
			continue
		}
		pubkeys = append(pubkeys, pk)
	}
	return pubkeys
}

func (m *TssVoter) GetSigners() []cosmos.AccAddress {
	addrs := make([]cosmos.AccAddress, 0)
	for _, a := range m.Signers {
		addr, err := cosmos.AccAddressFromBech32(a)
		if err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

// HasSigned - check if given address has signed
func (m *TssVoter) HasSigned(signer cosmos.AccAddress) bool {
	for _, sign := range m.GetSigners() {
		if sign.Equals(signer) {
			return true
		}
	}
	return false
}

// Sign this voter with given signer address
func (m *TssVoter) Sign(signer cosmos.AccAddress, chains []string, secp256k1Signature string) bool {
	if m.HasSigned(signer) {
		return false
	}
	for _, pk := range m.GetPubKeys() {
		addr, err := pk.GetThorAddress()
		if addr.Equals(signer) && err == nil {
			m.Signers = append(m.Signers, signer.String())
			m.Chains = append(m.Chains, chains...)
			if len(secp256k1Signature) > 0 {
				m.Secp256K1Signatures = append(m.Secp256K1Signatures, secp256k1Signature)
			}
			return true
		}
	}
	return false
}

// ConsensusChains - get a list of chains that have 2/3rds majority
func (m *TssVoter) ConsensusChains() common.Chains {
	chainCount := make(map[common.Chain]int)
	for _, chain := range m.GetChains() {
		if _, ok := chainCount[chain]; !ok {
			chainCount[chain] = 0
		}
		chainCount[chain]++
	}

	chains := make(common.Chains, 0)
	// analyze-ignore(map-iteration)
	for chain, count := range chainCount {
		if HasSuperMajority(count, len(m.PubKeys)) {
			chains = append(chains, chain)
		}
	}

	// sort chains for consistency
	sort.SliceStable(chains, func(i, j int) bool {
		return chains[i].String() < chains[j].String()
	})

	return chains
}

// HasCompleteConsensus return true only when all signers vote
func (m *TssVoter) HasCompleteConsensus() bool {
	return len(m.Signers) == len(m.PubKeys)
}

// ConsensusCheckSignature return true only when a consensus of members have provided
// an identical check signature.
func (m *TssVoter) ConsensusCheckSignature() (string, bool) {
	counts := make(map[string]int)
	for _, sig := range m.Secp256K1Signatures {
		if _, ok := counts[sig]; !ok {
			counts[sig] = 0
		}
		counts[sig]++
	}

	// analyze-ignore(map-iteration)
	for sig, count := range counts {
		if HasSuperMajority(count, len(m.PubKeys)) {
			return sig, true
		}
	}

	return "", false
}

// HasConsensus determine if this tss pool has enough signers
func (m *TssVoter) HasConsensus() bool {
	return HasSuperMajority(len(m.Signers), len(m.PubKeys))
}

// IsEmpty check whether TssVoter represent empty info
func (m *TssVoter) IsEmpty() bool {
	return len(m.ID) == 0 || len(m.PoolPubKey) == 0 || len(m.PubKeys) == 0
}

// String implement fmt.Stringer
func (m *TssVoter) String() string {
	return m.ID
}
