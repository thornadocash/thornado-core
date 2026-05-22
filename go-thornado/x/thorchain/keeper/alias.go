package keeper

import (
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

const (
	ModuleName   = types.ModuleName
	ReserveName  = types.ReserveName
	AsgardName   = types.AsgardName
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
	GetRandomValidatorNode   = types.GetRandomValidatorNode
	GetRandomTxHash          = types.GetRandomTxHash
	GetRandomBech32Addr      = types.GetRandomBech32Addr
	GetRandomPubKey          = types.GetRandomPubKey
)

type (
	MsgSwap = types.MsgSwap

	StreamingSwap            = types.StreamingSwap
	ObservedTxVoter          = types.ObservedTxVoter
	BanVoter                 = types.BanVoter
	ErrataTxVoter            = types.ErrataTxVoter
	TssVoter                 = types.TssVoter
	TssKeysignFailVoter      = types.TssKeysignFailVoter
	TssKeygenMetric          = types.TssKeygenMetric
	TssKeysignMetric         = types.TssKeysignMetric
	TxOutItem                = types.TxOutItem
	TxOut                    = types.TxOut
	KeygenBlock              = types.KeygenBlock
	ReserveContributors      = types.ReserveContributors
	Vault                    = types.Vault
	Vaults                   = types.Vaults
	Jail                     = types.Jail
	BondProvider             = types.BondProvider
	BondProviders            = types.BondProviders
	NodeAccount              = types.NodeAccount
	NodeAccounts             = types.NodeAccounts
	NodeMimirs               = types.NodeMimirs
	NodeStatus               = types.NodeStatus
	Network                  = types.Network
	ProtocolOwnedLiquidity   = types.ProtocolOwnedLiquidity
	POLReserveDeposit        = types.POLReserveDeposit
	VaultStatus              = types.VaultStatus
	NetworkFee               = types.NetworkFee
	ObservedNetworkFeeVoter  = types.ObservedNetworkFeeVoter
	RagnarokWithdrawPosition = types.RagnarokWithdrawPosition
	ChainContract            = types.ChainContract
	SolvencyVoter            = types.SolvencyVoter
	ReferenceMemo            = types.ReferenceMemo
	AffiliateFeeCollector    = types.AffiliateFeeCollector
	SwapperClout             = types.SwapperClout
	TradeAccount             = types.TradeAccount
	TradeUnit                = types.TradeUnit
	SecuredAsset             = types.SecuredAsset
	Upgrade                  = types.Upgrade
	PriceFeed                = types.PriceFeed
	OraclePrice              = types.OraclePrice
	Volume                   = types.Volume
	VolumeBucket             = types.VolumeBucket
)
