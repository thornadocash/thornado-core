package types

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// NewFrostKeysignFailVoter create a new instance of FrostKeysignFailVoter
func NewFrostKeysignFailVoter(id string, height int64) FrostKeysignFailVoter {
	return FrostKeysignFailVoter{
		ID:     id,
		Height: height,
	}
}

func (m *FrostKeysignFailVoter) GetSigners() []cosmos.AccAddress {
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
func (m *FrostKeysignFailVoter) HasSigned(signer cosmos.AccAddress) bool {
	for _, sign := range m.GetSigners() {
		if sign.Equals(signer) {
			return true
		}
	}
	return false
}

// Sign this voter with given signer address
func (m *FrostKeysignFailVoter) Sign(signer cosmos.AccAddress) bool {
	if m.HasSigned(signer) {
		return false
	}
	m.Signers = append(m.Signers, signer.String())
	return true
}

// MemberSignerCount returns the number of signers that are members of the given node accounts
func (m *FrostKeysignFailVoter) MemberSignerCount(nas NodeAccounts) int {
	count := 0
	for _, signer := range m.GetSigners() {
		for _, item := range nas {
			if signer.Equals(item.NodeAddress) {
				count++
				break
			}
		}
	}
	return count
}

// HasConsensus determine if this keygen vault has enough signers
func (m *FrostKeysignFailVoter) HasConsensus(nas NodeAccounts) bool {
	return HasSimpleMajority(m.MemberSignerCount(nas), len(nas))
}

// Empty to check whether this Voter is empty or not
func (m *FrostKeysignFailVoter) Empty() bool {
	return len(m.ID) == 0 || m.Height == 0
}

// String implement fmt.Stringer , return's the ID
func (m *FrostKeysignFailVoter) String() string {
	return m.ID
}
