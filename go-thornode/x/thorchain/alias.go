package thorchain

import (
	proto "github.com/cosmos/gogoproto/proto"

	"gitlab.com/thorchain/thornode/v3/common"
	mem "gitlab.com/thorchain/thornode/v3/x/thorchain/memo"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

const (
	ModuleName             = types.ModuleName
	ReserveName            = types.ReserveName
	AsgardName             = types.AsgardName
	BondName               = types.BondName
	LendingName            = types.LendingName
	AffiliateCollectorName = types.AffiliateCollectorName
	TreasuryName           = types.TreasuryName
	RUNEPoolName           = types.RUNEPoolName
	TCYClaimingName        = types.TCYClaimingName
	TCYStakeName           = types.TCYStakeName
	POLReserveName         = types.POLReserveName
	RouterKey              = types.RouterKey
	StoreKey               = types.StoreKey
	DefaultCodespace       = types.DefaultCodespace

	// pool status
	PoolAvailable = types.PoolStatus_Available
	PoolStaged    = types.PoolStatus_Staged
	PoolSuspended = types.PoolStatus_Suspended

	// Admin config keys
	MaxWithdrawBasisPoints = types.MaxWithdrawBasisPoints

	// Vaults
	AsgardVault   = types.VaultType_AsgardVault
	UnknownVault  = types.VaultType_UnknownVault
	ActiveVault   = types.VaultStatus_ActiveVault
	InactiveVault = types.VaultStatus_InactiveVault
	RetiringVault = types.VaultStatus_RetiringVault
	InitVault     = types.VaultStatus_InitVault

	// Node status
	NodeActive      = types.NodeStatus_Active
	NodeWhiteListed = types.NodeStatus_Whitelisted
	NodeDisabled    = types.NodeStatus_Disabled
	NodeReady       = types.NodeStatus_Ready
	NodeStandby     = types.NodeStatus_Standby
	NodeUnknown     = types.NodeStatus_Unknown

	// Node type
	NodeTypeUnknown   = types.NodeType_TypeUnknown
	NodeTypeValidator = types.NodeType_TypeValidator
	NodeTypeVault     = types.NodeType_TypeVault

	// Bond type
	BondPaid     = types.BondType_bond_paid
	BondReturned = types.BondType_bond_returned
	BondCost     = types.BondType_bond_cost
	BondReward   = types.BondType_bond_reward
	AsgardKeygen = types.KeygenType_AsgardKeygen

	// Bond type
	AddPendingLiquidity      = types.PendingLiquidityType_add
	WithdrawPendingLiquidity = types.PendingLiquidityType_withdraw

	// Swap Type
	MarketSwap = types.SwapType_market
	LimitSwap  = types.SwapType_limit

	// Advanced Swap Queue Mode
	AdvSwapQueueModeDisabled   = types.AdvSwapQueueModeDisabled
	AdvSwapQueueModeEnabled    = types.AdvSwapQueueModeEnabled
	AdvSwapQueueModeMarketOnly = types.AdvSwapQueueModeMarketOnly

	// Mint/Burn type
	MintSupplyType = types.MintBurnSupplyType_mint
	BurnSupplyType = types.MintBurnSupplyType_burn

	// Memos
	TxSwap              = mem.TxSwap
	TxLimitSwap         = mem.TxLimitSwap
	TxModifyLimitSwap   = mem.TxModifyLimitSwap
	TxAdd               = mem.TxAdd
	TxBond              = mem.TxBond
	TxMigrate           = mem.TxMigrate
	TxRagnarok          = mem.TxRagnarok
	TxReserve           = mem.TxReserve
	TxOutbound          = mem.TxOutbound
	TxRefund            = mem.TxRefund
	TxUnBond            = mem.TxUnbond
	TxRebond            = mem.TxRebond
	TxLeave             = mem.TxLeave
	TxMaint             = mem.TxMaint
	TxWithdraw          = mem.TxWithdraw
	TxTHORName          = mem.TxTHORName
	TxReferenceReadMemo = mem.TxReferenceReadMemo
	TxTCYClaim          = mem.TxTCYClaim
	TxTCYStake          = mem.TxTCYStake
	TxTCYUnstake        = mem.TxTCYUnstake
	TxOperatorRotate    = mem.TxOperatorRotate
)

var (
	NewPool                        = types.NewPool
	NewNetwork                     = types.NewNetwork
	NewProtocolOwnedLiquidity      = types.NewProtocolOwnedLiquidity
	NewPOLReserveDeposit           = types.NewPOLReserveDeposit
	NewRUNEPool                    = types.NewRUNEPool
	NewObservedTx                  = common.NewObservedTx
	NewTssVoter                    = types.NewTssVoter
	NewBanVoter                    = types.NewBanVoter
	NewErrataTxVoter               = types.NewErrataTxVoter
	NewObservedTxVoter             = types.NewObservedTxVoter
	NewMsgRunePoolDeposit          = types.NewMsgRunePoolDeposit
	NewMsgRunePoolWithdraw         = types.NewMsgRunePoolWithdraw
	NewMsgTradeAccountDeposit      = types.NewMsgTradeAccountDeposit
	NewMsgTradeAccountWithdrawal   = types.NewMsgTradeAccountWithdrawal
	NewMsgSecuredAssetDeposit      = types.NewMsgSecuredAssetDeposit
	NewMsgSecuredAssetWithdraw     = types.NewMsgSecuredAssetWithdraw
	NewMsgMimir                    = types.NewMsgMimir
	NewMsgNodePauseChain           = types.NewMsgNodePauseChain
	NewMsgDeposit                  = types.NewMsgDeposit
	NewMsgTssPool                  = types.NewMsgTssPool
	NewMsgTssPoolV2                = types.NewMsgTssPoolV2
	NewMsgTssKeysignFail           = types.NewMsgTssKeysignFail
	NewMsgObservedTxIn             = types.NewMsgObservedTxIn
	NewMsgObservedTxOut            = types.NewMsgObservedTxOut
	NewMsgNoOp                     = types.NewMsgNoOp
	NewMsgConsolidate              = types.NewMsgConsolidate
	NewMsgDonate                   = types.NewMsgDonate
	NewMsgAddLiquidity             = types.NewMsgAddLiquidity
	NewMsgWithdrawLiquidity        = types.NewMsgWithdrawLiquidity
	NewMsgSwap                     = types.NewMsgSwap
	NewMsgModifyLimitSwap          = types.NewMsgModifyLimitSwap
	NewKeygen                      = types.NewKeygen
	NewKeygenBlock                 = types.NewKeygenBlock
	NewMsgSetNodeKeys              = types.NewMsgSetNodeKeys
	NewMsgManageTHORName           = types.NewMsgManageTHORName
	NewMsgReferenceMemo            = types.NewMsgReferenceMemo
	NewMsgSwitch                   = types.NewMsgSwitch
	NewMsgOperatorRotate           = types.NewMsgOperatorRotate
	NewMsgPriceFeedQuorumBatch     = types.NewMsgPriceFeedQuorumBatch
	NewTxOut                       = types.NewTxOut
	NewEventRewards                = types.NewEventRewards
	NewEventPOLReserveDeploy       = types.NewEventPOLReserveDeploy
	NewEventPool                   = types.NewEventPool
	NewEventDonate                 = types.NewEventDonate
	NewEventSwap                   = types.NewEventSwap
	NewEventAffiliateFee           = types.NewEventAffiliateFee
	NewEventStreamingSwap          = types.NewEventStreamingSwap
	NewEventLimitSwap              = types.NewEventLimitSwap
	NewEventModifyLimitSwap        = types.NewEventModifyLimitSwap
	NewEventLimitSwapClose         = types.NewEventLimitSwapClose
	NewEventAddLiquidity           = types.NewEventAddLiquidity
	NewEventWithdraw               = types.NewEventWithdraw
	NewEventRefund                 = types.NewEventRefund
	NewEventBond                   = types.NewEventBond
	NewEventReBond                 = types.NewEventReBond
	NewEventGas                    = types.NewEventGas
	NewEventScheduledOutbound      = types.NewEventScheduledOutbound
	NewEventSecurity               = types.NewEventSecurity
	NewEventSlash                  = types.NewEventSlash
	NewEventSlashPoint             = types.NewEventSlashPoint
	NewEventReserve                = types.NewEventReserve
	NewEventErrata                 = types.NewEventErrata
	NewEventFee                    = types.NewEventFee
	NewEventOutbound               = types.NewEventOutbound
	NewEventSetMimir               = types.NewEventSetMimir
	NewEventSetNodeMimir           = types.NewEventSetNodeMimir
	NewEventTssKeygenSuccess       = types.NewEventTssKeygenSuccess
	NewEventTssKeygenFailure       = types.NewEventTssKeygenFailure
	NewEventTssKeygenMetric        = types.NewEventTssKeygenMetric
	NewEventTssKeysignMetric       = types.NewEventTssKeysignMetric
	NewEventPoolBalanceChanged     = types.NewEventPoolBalanceChanged
	NewEventPendingLiquidity       = types.NewEventPendingLiquidity
	NewEventTHORName               = types.NewEventTHORName
	NewEventMintBurn               = types.NewEventMintBurn
	NewEventVersion                = types.NewEventVersion
	NewEventTradeAccountDeposit    = types.NewEventTradeAccountDeposit
	NewEventTradeAccountWithdraw   = types.NewEventTradeAccountWithdraw
	NewEventSecuredAssetDeposit    = types.NewEventSecuredAssetDeposit
	NewEventSecuredAssetWithdraw   = types.NewEventSecuredAssetWithdraw
	NewEventRUNEPoolDeposit        = types.NewEventRUNEPoolDeposit
	NewEventRUNEPoolWithdraw       = types.NewEventRUNEPoolWithdraw
	NewEventSwitch                 = types.NewEventSwitch
	NewEventOperatorRotate         = types.NewEventOperatorRotate
	NewEventOraclePrice            = types.NewEventOraclePrice
	NewEventFailedOutboundRecovery = types.NewEventFailedOutboundRecovery
	NewPoolMod                     = types.NewPoolMod
	NewMsgRefundTx                 = types.NewMsgRefundTx
	NewMsgOutboundTx               = types.NewMsgOutboundTx
	NewMsgMigrate                  = types.NewMsgMigrate
	NewMsgRagnarok                 = types.NewMsgRagnarok
	ModuleCdc                      = types.ModuleCdc
	RegisterLegacyAminoCodec       = types.RegisterLegacyAminoCodec
	RegisterInterfaces             = types.RegisterInterfaces
	NewBondProviders               = types.NewBondProviders
	NewBondProvider                = types.NewBondProvider
	NewNodeAccount                 = types.NewNodeAccount
	NewVault                       = types.NewVault
	NewVaultV2                     = types.NewVaultV2
	NewReserveContributor          = types.NewReserveContributor
	NewMsgReserveContributor       = types.NewMsgReserveContributor
	NewMsgBond                     = types.NewMsgBond
	NewMsgUnBond                   = types.NewMsgUnBond
	NewMsgReBond                   = types.NewMsgReBond
	NewMsgErrataTx                 = types.NewMsgErrataTx
	NewMsgBan                      = types.NewMsgBan
	NewMsgLeave                    = types.NewMsgLeave
	NewMsgSetVersion               = types.NewMsgSetVersion
	NewMsgProposeUpgrade           = types.NewMsgProposeUpgrade
	NewMsgApproveUpgrade           = types.NewMsgApproveUpgrade
	NewMsgRejectUpgrade            = types.NewMsgRejectUpgrade
	NewMsgSetIPAddress             = types.NewMsgSetIPAddress
	NewMsgNetworkFee               = types.NewMsgNetworkFee
	NewNetworkFee                  = types.NewNetworkFee
	NewTHORName                    = types.NewTHORName
	NewReferenceMemo               = types.NewReferenceMemo
	NewStreamingSwap               = types.NewStreamingSwap
	GetPoolStatus                  = types.GetPoolStatus
	GetRandomVault                 = types.GetRandomVault
	GetRandomTx                    = types.GetRandomTx
	GetRandomObservedTx            = types.GetRandomObservedTx
	GetRandomTxOutItem             = types.GetRandomTxOutItem
	GetRandomObservedTxVoter       = types.GetRandomObservedTxVoter
	GetRandomValidatorNode         = types.GetRandomValidatorNode
	GetRandomVaultNode             = types.GetRandomVaultNode
	GetRandomTHORAddress           = types.GetRandomTHORAddress
	GetRandomRUNEAddress           = types.GetRandomRUNEAddress
	GetRandomETHAddress            = types.GetRandomETHAddress
	GetRandomGAIAAddress           = types.GetRandomGAIAAddress
	GetRandomBTCAddress            = types.GetRandomBTCAddress
	GetRandomLTCAddress            = types.GetRandomLTCAddress
	GetRandomDOGEAddress           = types.GetRandomDOGEAddress
	GetRandomBCHAddress            = types.GetRandomBCHAddress
	GetRandomTxHash                = types.GetRandomTxHash
	GetRandomBech32Addr            = types.GetRandomBech32Addr
	GetRandomBech32ConsensusPubKey = types.GetRandomBech32ConsensusPubKey
	GetRandomPubKey                = types.GetRandomPubKey
	GetRandomPubKeySet             = types.GetRandomPubKeySet
	GetCurrentVersion              = types.GetCurrentVersion
	SetupConfigForTest             = types.SetupConfigForTest
	HasSimpleMajority              = types.HasSimpleMajority
	HasSuperMajority               = types.HasSuperMajority
	HasMinority                    = types.HasMinority
	DefaultGenesis                 = types.DefaultGenesis
	NewSolvencyVoter               = types.NewSolvencyVoter
	NewMsgSolvency                 = types.NewMsgSolvency
	NewSwapperClout                = types.NewSwapperClout
	NewMsgTCYClaim                 = types.NewMsgTCYClaim
	NewMsgTCYStake                 = types.NewMsgTCYStake
	NewMsgTCYUnstake               = types.NewMsgTCYUnstake

	// Memo
	ParseMemo              = mem.ParseMemo
	ParseMemoWithTHORNames = mem.ParseMemoWithTHORNames
	FetchAddress           = mem.FetchAddress
	NewRefundMemo          = mem.NewRefundMemo
	NewOutboundMemo        = mem.NewOutboundMemo
	NewRagnarokMemo        = mem.NewRagnarokMemo
	NewMigrateMemo         = mem.NewMigrateMemo
	NewReferenceReadMemo   = mem.NewReferenceReadMemo
)

type (
	// Msgs
	MsgSend                   = types.MsgSend
	MsgDeposit                = types.MsgDeposit
	MsgBond                   = types.MsgBond
	MsgUnBond                 = types.MsgUnBond
	MsgReBond                 = types.MsgReBond
	MsgNoOp                   = types.MsgNoOp
	MsgTradeAccountDeposit    = types.MsgTradeAccountDeposit
	MsgTradeAccountWithdrawal = types.MsgTradeAccountWithdrawal
	MsgSecuredAssetDeposit    = types.MsgSecuredAssetDeposit
	MsgSecuredAssetWithdraw   = types.MsgSecuredAssetWithdraw
	MsgConsolidate            = types.MsgConsolidate
	MsgDonate                 = types.MsgDonate
	MsgWithdrawLiquidity      = types.MsgWithdrawLiquidity
	MsgAddLiquidity           = types.MsgAddLiquidity
	MsgOutboundTx             = types.MsgOutboundTx
	MsgMimir                  = types.MsgMimir
	MsgNodePauseChain         = types.MsgNodePauseChain
	MsgMigrate                = types.MsgMigrate
	MsgRagnarok               = types.MsgRagnarok
	MsgRefundTx               = types.MsgRefundTx
	MsgErrataTx               = types.MsgErrataTx
	MsgBan                    = types.MsgBan
	MsgSwap                   = types.MsgSwap
	MsgModifyLimitSwap        = types.MsgModifyLimitSwap
	MsgSetVersion             = types.MsgSetVersion
	MsgProposeUpgrade         = types.MsgProposeUpgrade
	MsgApproveUpgrade         = types.MsgApproveUpgrade
	MsgRejectUpgrade          = types.MsgRejectUpgrade
	MsgSetIPAddress           = types.MsgSetIPAddress
	MsgSetNodeKeys            = types.MsgSetNodeKeys
	MsgLeave                  = types.MsgLeave
	MsgMaint                  = types.MsgMaint
	MsgReserveContributor     = types.MsgReserveContributor
	MsgObservedTxIn           = types.MsgObservedTxIn
	MsgObservedTxOut          = types.MsgObservedTxOut
	MsgTssPool                = types.MsgTssPool
	MsgTssKeysignFail         = types.MsgTssKeysignFail
	MsgNetworkFee             = types.MsgNetworkFee
	MsgManageTHORName         = types.MsgManageTHORName
	MsgReferenceMemo          = types.MsgReferenceMemo
	MsgSolvency               = types.MsgSolvency
	MsgRunePoolDeposit        = types.MsgRunePoolDeposit
	MsgRunePoolWithdraw       = types.MsgRunePoolWithdraw
	MsgSwitch                 = types.MsgSwitch
	MsgTCYClaim               = types.MsgTCYClaim
	MsgTCYStake               = types.MsgTCYStake
	MsgTCYUnstake             = types.MsgTCYUnstake
	MsgOperatorRotate         = types.MsgOperatorRotate

	// Keeper structs
	PoolStatus               = types.PoolStatus
	Pool                     = types.Pool
	Pools                    = types.Pools
	LiquidityProvider        = types.LiquidityProvider
	LiquidityProviders       = types.LiquidityProviders
	StreamingSwap            = types.StreamingSwap
	StreamingSwaps           = types.StreamingSwaps
	ObservedTxs              = common.ObservedTxs
	ObservedTx               = common.ObservedTx
	ObservedTxVoter          = types.ObservedTxVoter
	ObservedTxVoters         = types.ObservedTxVoters
	BanVoter                 = types.BanVoter
	ErrataTxVoter            = types.ErrataTxVoter
	TssVoter                 = types.TssVoter
	TssKeysignFailVoter      = types.TssKeysignFailVoter
	TxOutItem                = types.TxOutItem
	TxOut                    = types.TxOut
	Keygen                   = types.Keygen
	KeygenBlock              = types.KeygenBlock
	EventSwap                = types.EventSwap
	EventAffiliateFee        = types.EventAffiliateFee
	EventAddLiquidity        = types.EventAddLiquidity
	EventWithdraw            = types.EventWithdraw
	EventDonate              = types.EventDonate
	EventRewards             = types.EventRewards
	EventErrata              = types.EventErrata
	EventReserve             = types.EventReserve
	PoolAmt                  = types.PoolAmt
	PoolMod                  = types.PoolMod
	PoolMods                 = types.PoolMods
	ReserveContributor       = types.ReserveContributor
	ReserveContributors      = types.ReserveContributors
	Vault                    = types.Vault
	Vaults                   = types.Vaults
	NodeAccount              = types.NodeAccount
	NodeAccounts             = types.NodeAccounts
	NodeStatus               = types.NodeStatus
	BondProviders            = types.BondProviders
	BondProvider             = types.BondProvider
	Network                  = types.Network
	ProtocolOwnedLiquidity   = types.ProtocolOwnedLiquidity
	POLReserveDeposit        = types.POLReserveDeposit
	VaultStatus              = types.VaultStatus
	GasPool                  = types.GasPool
	EventGas                 = types.EventGas
	EventPool                = types.EventPool
	EventRefund              = types.EventRefund
	EventBond                = types.EventBond
	EventFee                 = types.EventFee
	EventSlash               = types.EventSlash
	EventOutbound            = types.EventOutbound
	NetworkFee               = types.NetworkFee
	PriceFeed                = types.PriceFeed
	OraclePrice              = types.OraclePrice
	ObservedNetworkFeeVoter  = types.ObservedNetworkFeeVoter
	Jail                     = types.Jail
	RagnarokWithdrawPosition = types.RagnarokWithdrawPosition
	ChainContract            = types.ChainContract
	Blame                    = types.Blame
	Node                     = types.Node
	ReferenceMemo            = types.ReferenceMemo
	THORName                 = types.THORName
	THORNameAlias            = types.THORNameAlias
	AffiliateFeeCollector    = types.AffiliateFeeCollector
	NodeMimir                = types.NodeMimir
	NodeMimirs               = types.NodeMimirs
	SwapperClout             = types.SwapperClout
	TradeAccount             = types.TradeAccount
	TradeUnit                = types.TradeUnit
	SecuredAsset             = types.SecuredAsset
	RUNEProvider             = types.RUNEProvider
	RUNEPool                 = types.RUNEPool
	TCYClaimer               = types.TCYClaimer
	TCYStaker                = types.TCYStaker
	VolumeBucket             = types.VolumeBucket
	ContractInfo             = types.ContractInfo

	// Memo
	RefundMemo         = mem.RefundMemo
	MigrateMemo        = mem.MigrateMemo
	RagnarokMemo       = mem.RagnarokMemo
	BondMemo           = mem.BondMemo
	UnbondMemo         = mem.UnbondMemo
	RebondMemo         = mem.RebondMemo
	OutboundMemo       = mem.OutboundMemo
	LeaveMemo          = mem.LeaveMemo
	MaintMemo          = mem.MaintMemo
	NoOpMemo           = mem.NoOpMemo
	ConsolidateMemo    = mem.ConsolidateMemo
	ReferenceWriteMemo = mem.ReferenceWriteMemo
	ReferenceReadMemo  = mem.ReferenceReadMemo
	OperatorRotateMemo = mem.OperatorRotateMemo

	// Proto
	ProtoStrings = types.ProtoStrings
	ProtoInt64   = types.ProtoInt64
)

var _ proto.Message = &types.LiquidityProvider{}
