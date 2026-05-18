package types

import "gitlab.com/thorchain/thornode/v3/common"

// NewVault create a new instance of vault
func NewVaultV2(height int64, status VaultStatus, vtype VaultType, ecdsaPubKey common.PubKey, chains []string, routers []ChainContract, eddsaPubKey common.PubKey) Vault {
	return Vault{
		BlockHeight: height,
		StatusSince: height,
		PubKey:      ecdsaPubKey,
		Coins:       make(common.Coins, 0),
		Type:        vtype,
		Status:      status,
		Chains:      chains,
		Routers:     routers,
		PubKeyEddsa: eddsaPubKey,
	}
}
