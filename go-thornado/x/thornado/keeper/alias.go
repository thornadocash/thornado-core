package keeper

import (
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	ModuleName  = types.ModuleName
	ReserveName = types.ReserveName
	BaseName    = types.BaseName
	BondName    = types.BondName
	StoreKey    = types.StoreKey

	ActiveVault = types.VaultStatus_ActiveVault

	// Node status
	NodeActive = types.NodeStatus_Active
)

var (
	NewJail                  = types.NewJail
	ModuleCdc                = types.ModuleCdc
	RegisterLegacyAminoCodec = types.RegisterLegacyAminoCodec
	GetRandomVault           = types.GetRandomVault
	GetRandomNode            = types.GetRandomNode
	GetRandomTxHash          = types.GetRandomTxHash
	GetRandomBech32Addr      = types.GetRandomBech32Addr
	GetRandomPubKey          = types.GetRandomPubKey
)

type (
	ObservedTxVoter         = types.ObservedTxVoter
	ErrataTxVoter           = types.ErrataTxVoter
	FrostVoter              = types.FrostVoter
	FrostKeysignFailVoter   = types.FrostKeysignFailVoter
	FrostKeygenMetric       = types.FrostKeygenMetric
	FrostKeysignMetric      = types.FrostKeysignMetric
	TxOutItem               = types.TxOutItem
	TxOut                   = types.TxOut
	KeygenBlock             = types.KeygenBlock
	Vault                   = types.Vault
	Vaults                  = types.Vaults
	Jail                    = types.Jail
	NodeAccount             = types.NodeAccount
	NodeAccounts            = types.NodeAccounts
	NodeConfigs             = types.NodeConfigs
	StoreMigrateVotes       = types.StoreMigrateVotes
	NodeStatus              = types.NodeStatus
	Network                 = types.Network
	VaultStatus             = types.VaultStatus
	NetworkFee              = types.NetworkFee
	ObservedNetworkFeeVoter = types.ObservedNetworkFeeVoter
	SolvencyVoter           = types.SolvencyVoter
	Upgrade                 = types.Upgrade
	OraclePrice             = types.OraclePrice
)
