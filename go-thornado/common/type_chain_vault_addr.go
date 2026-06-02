package common

import "fmt"

// ChainVaultInfo represent the vault address specific for a chain
type ChainVaultInfo struct {
	Chain        Chain   `json:"chain"`
	PubKey       PubKey  `json:"pub_key"`
	VaultAddress Address `json:"vault_address"`
}

// NoChainVaultInfo everything is empty
var NoChainVaultInfo ChainVaultInfo

// NewChainVaultInfo create a new instance of ChainVaultInfo
func NewChainVaultInfo(chain Chain, pubKey PubKey) (ChainVaultInfo, error) {
	if chain.IsEmpty() {
		return NoChainVaultInfo, fmt.Errorf("chain is empty")
	}
	if pubKey.IsEmpty() {
		return NoChainVaultInfo, fmt.Errorf("pubkey is empty")
	}

	addr, err := pubKey.GetAddress(chain)
	if err != nil {
		return NoChainVaultInfo, fmt.Errorf("fail to get address for chain %s,%w", chain, err)
	}

	return ChainVaultInfo{
		Chain:        chain,
		PubKey:       pubKey,
		VaultAddress: addr,
	}, nil
}

// IsEmpty whether the struct is empty
func (cpi ChainVaultInfo) IsEmpty() bool {
	return cpi.Chain.IsEmpty() || cpi.PubKey.IsEmpty()
}
