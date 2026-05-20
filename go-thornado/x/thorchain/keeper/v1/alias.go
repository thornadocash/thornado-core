package keeperv1

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

const (
	ModuleName             = types.ModuleName
	ReserveName            = types.ReserveName
	AsgardName             = types.AsgardName
	AffiliateCollectorName = types.AffiliateCollectorName
	TreasuryName           = types.TreasuryName
	RUNEPoolName           = types.RUNEPoolName
	TCYClaimingName        = types.TCYClaimingName
	TCYStakeName           = types.TCYStakeName
	POLReserveName         = types.POLReserveName
	BondName               = types.BondName
	LendingName            = types.LendingName
	StoreKey               = types.StoreKey

	// Vaults
	AsgardVault   = types.VaultType_AsgardVault
	ActiveVault   = types.VaultStatus_ActiveVault
	InitVault     = types.VaultStatus_InitVault
	RetiringVault = types.VaultStatus_RetiringVault
	InactiveVault = types.VaultStatus_InactiveVault

	// Node status
	NodeActive  = types.NodeStatus_Active
	NodeStandby = types.NodeStatus_Standby
	NodeUnknown = types.NodeStatus_Unknown

	// Node type
	NodeTypeUnknown   = types.NodeType_TypeUnknown
	NodeTypeValidator = types.NodeType_TypeValidator
	NodeTypeVault     = types.NodeType_TypeVault

	// Mint/Burn type
	MintSupplyType = types.MintBurnSupplyType_mint
	BurnSupplyType = types.MintBurnSupplyType_burn

	// Bond type
	AsgardKeygen = types.KeygenType_AsgardKeygen
	BondCost     = types.BondType_bond_cost
	BondReturned = types.BondType_bond_returned
)

var (
	NewPool                    = types.NewPool
	NewJail                    = types.NewJail
	NewStreamingSwap           = types.NewStreamingSwap
	NewNetwork                 = types.NewNetwork
	NewProtocolOwnedLiquidity  = types.NewProtocolOwnedLiquidity
	NewPOLReserveDeposit       = types.NewPOLReserveDeposit
	NewRUNEPool                = types.NewRUNEPool
	NewObservedTx              = common.NewObservedTx
	NewTssVoter                = types.NewTssVoter
	NewBanVoter                = types.NewBanVoter
	NewErrataTxVoter           = types.NewErrataTxVoter
	NewObservedTxVoter         = types.NewObservedTxVoter
	NewKeygen                  = types.NewKeygen
	NewKeygenBlock             = types.NewKeygenBlock
	NewTxOut                   = types.NewTxOut
	HasSuperMajority           = types.HasSuperMajority
	RegisterLegacyAminoCodec   = types.RegisterLegacyAminoCodec
	NewTradeAccount            = types.NewTradeAccount
	NewTradeUnit               = types.NewTradeUnit
	NewSecuredAsset            = types.NewSecuredAsset
	NewRUNEProvider            = types.NewRUNEProvider
	NewNodeAccount             = types.NewNodeAccount
	NewBondProviders           = types.NewBondProviders
	NewBondProvider            = types.NewBondProvider
	NewVault                   = types.NewVault
	NewVaultV2                 = types.NewVaultV2
	NewReserveContributor      = types.NewReserveContributor
	NewReferenceMemo           = types.NewReferenceMemo
	NewTHORName                = types.NewTHORName
	NewEventBond               = types.NewEventBond
	NewEventMintBurn           = types.NewEventMintBurn
	GetRandomTx                = types.GetRandomTx
	GetRandomValidatorNode     = types.GetRandomValidatorNode
	GetRandomVaultNode         = types.GetRandomVaultNode
	GetRandomBTCAddress        = types.GetRandomBTCAddress
	GetRandomETHAddress        = types.GetRandomETHAddress
	GetRandomBCHAddress        = types.GetRandomBCHAddress
	GetRandomRUNEAddress       = types.GetRandomRUNEAddress
	GetRandomTHORAddress       = types.GetRandomTHORAddress
	GetRandomTxHash            = types.GetRandomTxHash
	GetRandomBech32Addr        = types.GetRandomBech32Addr
	GetRandomPubKey            = types.GetRandomPubKey
	GetRandomPubKeySet         = types.GetRandomPubKeySet
	GetCurrentVersion          = types.GetCurrentVersion
	NewObservedNetworkFeeVoter = types.NewObservedNetworkFeeVoter
	NewNetworkFee              = types.NewNetworkFee
	NewTssKeysignFailVoter     = types.NewTssKeysignFailVoter
	SetupConfigForTest         = types.SetupConfigForTest
	NewChainContract           = types.NewChainContract
	NewSwapperClout            = types.NewSwapperClout
	NewTCYClaimer              = types.NewTCYClaimer
	NewTCYStaker               = types.NewTCYStaker
)

type (
	MsgSwap                  = types.MsgSwap
	MsgAddLiquidity          = types.MsgAddLiquidity
	MsgWithdrawLiquidity     = types.MsgWithdrawLiquidity
	Pool                     = types.Pool
	Pools                    = types.Pools
	LiquidityProvider        = types.LiquidityProvider
	StreamingSwap            = types.StreamingSwap
	ObservedTxs              = common.ObservedTxs
	ObservedTxVoter          = types.ObservedTxVoter
	BanVoter                 = types.BanVoter
	ErrataTxVoter            = types.ErrataTxVoter
	TssVoter                 = types.TssVoter
	TssKeysignFailVoter      = types.TssKeysignFailVoter
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
	NodeStatus               = types.NodeStatus
	NodeType                 = types.NodeType
	Network                  = types.Network
	VaultStatus              = types.VaultStatus
	NetworkFee               = types.NetworkFee
	ObservedNetworkFeeVoter  = types.ObservedNetworkFeeVoter
	RagnarokWithdrawPosition = types.RagnarokWithdrawPosition
	TssKeygenMetric          = types.TssKeygenMetric
	TssKeysignMetric         = types.TssKeysignMetric
	ChainContract            = types.ChainContract
	ReferenceMemo            = types.ReferenceMemo
	THORName                 = types.THORName
	THORNameAlias            = types.THORNameAlias
	AffiliateFeeCollector    = types.AffiliateFeeCollector
	SolvencyVoter            = types.SolvencyVoter
	MinJoinLast              = types.MinJoinLast
	NodeMimir                = types.NodeMimir
	NodeMimirs               = types.NodeMimirs
	ProtocolOwnedLiquidity   = types.ProtocolOwnedLiquidity
	POLReserveDeposit        = types.POLReserveDeposit
	SwapperClout             = types.SwapperClout
	TradeAccount             = types.TradeAccount
	TradeUnit                = types.TradeUnit
	SecuredAsset             = types.SecuredAsset
	RUNEProvider             = types.RUNEProvider
	RUNEPool                 = types.RUNEPool
	MsgTCYClaim              = types.MsgTCYClaim
	MsgTCYStake              = types.MsgTCYStake
	MsgTCYUnstake            = types.MsgTCYUnstake
	TCYClaimer               = types.TCYClaimer
	TCYStaker                = types.TCYStaker
	OraclePrice              = types.OraclePrice
	PriceFeed                = types.PriceFeed
	Volume                   = types.Volume
	VolumeBucket             = types.VolumeBucket

	ProtoInt64        = types.ProtoInt64
	ProtoUint64       = types.ProtoUint64
	ProtoAccAddress   = types.ProtoAccAddress
	ProtoAccAddresses = types.ProtoAccAddresses
	ProtoString       = types.ProtoString
	ProtoStrings      = types.ProtoStrings
	ProtoUint         = common.ProtoUint
	ProtoBools        = types.ProtoBools
)
