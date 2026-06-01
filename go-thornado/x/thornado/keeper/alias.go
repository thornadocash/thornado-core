package keeper

import (
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	ModuleName   = types.ModuleName
	ReserveName  = types.ReserveName
	BaseName     = types.BaseName
	TreasuryName = types.TreasuryName
	BondName     = types.BondName
	StoreKey     = types.StoreKey

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
	BanVoter                = types.BanVoter
	ErrataTxVoter           = types.ErrataTxVoter
	TssVoter                = types.TssVoter
	TssKeysignFailVoter     = types.TssKeysignFailVoter
	TssKeygenMetric         = types.TssKeygenMetric
	TssKeysignMetric        = types.TssKeysignMetric
	TxOutItem               = types.TxOutItem
	TxOut                   = types.TxOut
	KeygenBlock             = types.KeygenBlock
	Vault                   = types.Vault
	Vaults                  = types.Vaults
	Jail                    = types.Jail
	NodeAccount             = types.NodeAccount
	NodeAccounts            = types.NodeAccounts
	NodeConfigs             = types.NodeConfigs
	NodeStatus              = types.NodeStatus
	Network                 = types.Network
	VaultStatus             = types.VaultStatus
	NetworkFee              = types.NetworkFee
	ObservedNetworkFeeVoter = types.ObservedNetworkFeeVoter
	SolvencyVoter           = types.SolvencyVoter
	Upgrade                 = types.Upgrade
	PriceFeed               = types.PriceFeed
	OraclePrice             = types.OraclePrice
)
