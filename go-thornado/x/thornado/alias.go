package thornado

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	ModuleName       = types.ModuleName
	ReserveName      = types.ReserveName
	AsgardName       = types.AsgardName
	BondName         = types.BondName
	LendingName      = types.LendingName
	TreasuryName     = types.TreasuryName
	RouterKey        = types.RouterKey
	StoreKey         = types.StoreKey
	DefaultCodespace = types.DefaultCodespace

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
	NodeSelected    = types.NodeStatus_Selected
	NodeStandby     = types.NodeStatus_Standby
	NodeUnknown     = types.NodeStatus_Unknown

	// Node type
	NodeTypeUnknown = types.NodeType_TypeUnknown
	NodeTypeNode    = types.NodeType_TypeNode
	NodeTypeVault   = types.NodeType_TypeVault

	// Bond type
	BondPaid     = types.BondType_bond_paid
	BondReturned = types.BondType_bond_returned
	BondCost     = types.BondType_bond_cost
	BondReward   = types.BondType_bond_reward
	AsgardKeygen = types.KeygenType_AsgardKeygen

	// Mint/Burn type
	MintSupplyType = types.MintBurnSupplyType_mint
	BurnSupplyType = types.MintBurnSupplyType_burn
)

var (
	NewNetwork                      = types.NewNetwork
	NewObservedTx                   = common.NewObservedTx
	NewTssVoter                     = types.NewTssVoter
	NewBanVoter                     = types.NewBanVoter
	NewErrataTxVoter                = types.NewErrataTxVoter
	NewObservedTxVoter              = types.NewObservedTxVoter
	NewMsgMimir                     = types.NewMsgMimir
	NewMsgNodePauseChain            = types.NewMsgNodePauseChain
	NewMsgDeposit                   = types.NewMsgDeposit
	NewMsgTssPool                   = types.NewMsgTssPool
	NewMsgTssPoolV2                 = types.NewMsgTssPoolV2
	NewMsgTssKeysignFail            = types.NewMsgTssKeysignFail
	NewMsgObservedTxIn              = types.NewMsgObservedTxIn
	NewMsgObservedTxOut             = types.NewMsgObservedTxOut
	NewMsgNoOp                      = types.NewMsgNoOp
	NewMsgConsolidate               = types.NewMsgConsolidate
	NewKeygen                       = types.NewKeygen
	NewKeygenBlock                  = types.NewKeygenBlock
	NewMsgSetNodeKeys               = types.NewMsgSetNodeKeys
	NewMsgOperatorRotate            = types.NewMsgOperatorRotate
	NewMsgPriceFeedQuorumBatch      = types.NewMsgPriceFeedQuorumBatch
	NewTxOut                        = types.NewTxOut
	NewEventRewards                 = types.NewEventRewards
	NewEventPool                    = types.NewEventPool
	NewEventDonate                  = types.NewEventDonate
	NewEventSwap                    = types.NewEventSwap
	NewEventLimitSwap               = types.NewEventLimitSwap
	NewEventModifyLimitSwap         = types.NewEventModifyLimitSwap
	NewEventLimitSwapClose          = types.NewEventLimitSwapClose
	NewEventRefund                  = types.NewEventRefund
	NewEventBond                    = types.NewEventBond
	NewEventReBond                  = types.NewEventReBond
	NewEventGas                     = types.NewEventGas
	NewEventScheduledOutbound       = types.NewEventScheduledOutbound
	NewEventSecurity                = types.NewEventSecurity
	NewEventSlash                   = types.NewEventSlash
	NewEventSlashPoint              = types.NewEventSlashPoint
	NewEventErrata                  = types.NewEventErrata
	NewEventFee                     = types.NewEventFee
	NewEventOutbound                = types.NewEventOutbound
	NewEventSetMimir                = types.NewEventSetMimir
	NewEventSetNodeMimir            = types.NewEventSetNodeMimir
	NewEventTssKeygenSuccess        = types.NewEventTssKeygenSuccess
	NewEventTssKeygenFailure        = types.NewEventTssKeygenFailure
	NewEventTssKeygenMetric         = types.NewEventTssKeygenMetric
	NewEventTssKeysignMetric        = types.NewEventTssKeysignMetric
	NewEventPoolBalanceChanged      = types.NewEventPoolBalanceChanged
	NewEventMintBurn                = types.NewEventMintBurn
	NewEventVersion                 = types.NewEventVersion
	NewEventOperatorRotate          = types.NewEventOperatorRotate
	NewEventOraclePrice             = types.NewEventOraclePrice
	NewEventFailedOutboundRecovery  = types.NewEventFailedOutboundRecovery
	NewPoolMod                      = types.NewPoolMod
	NewMsgRefundTx                  = types.NewMsgRefundTx
	NewMsgOutboundTx                = types.NewMsgOutboundTx
	NewMsgMigrate                   = types.NewMsgMigrate
	ModuleCdc                       = types.ModuleCdc
	RegisterLegacyAminoCodec        = types.RegisterLegacyAminoCodec
	RegisterInterfaces              = types.RegisterInterfaces
	NewNodeAccount                  = types.NewNodeAccount
	NewVault                        = types.NewVault
	NewVaultV2                      = types.NewVaultV2
	NewMsgErrataTx                  = types.NewMsgErrataTx
	NewMsgBan                       = types.NewMsgBan
	NewMsgSetVersion                = types.NewMsgSetVersion
	NewMsgProposeUpgrade            = types.NewMsgProposeUpgrade
	NewMsgApproveUpgrade            = types.NewMsgApproveUpgrade
	NewMsgRejectUpgrade             = types.NewMsgRejectUpgrade
	NewMsgSetIPAddress              = types.NewMsgSetIPAddress
	NewMsgNetworkFee                = types.NewMsgNetworkFee
	NewNetworkFee                   = types.NewNetworkFee
	GetRandomVault                  = types.GetRandomVault
	GetRandomTx                     = types.GetRandomTx
	GetRandomObservedTx             = types.GetRandomObservedTx
	GetRandomTxOutItem              = types.GetRandomTxOutItem
	GetRandomObservedTxVoter        = types.GetRandomObservedTxVoter
	GetRandomNode                   = types.GetRandomNode
	GetRandomVaultNode              = types.GetRandomVaultNode
	GetRandomTHORAddress            = types.GetRandomTHORAddress
	GetRandomRUNEAddress            = types.GetRandomRUNEAddress
	GetRandomETHAddress             = types.GetRandomETHAddress
	GetRandomGAIAAddress            = types.GetRandomGAIAAddress
	GetRandomBTCAddress             = types.GetRandomBTCAddress
	GetRandomLTCAddress             = types.GetRandomLTCAddress
	GetRandomDOGEAddress            = types.GetRandomDOGEAddress
	GetRandomBCHAddress             = types.GetRandomBCHAddress
	GetRandomTxHash                 = types.GetRandomTxHash
	GetRandomBech32Addr             = types.GetRandomBech32Addr
	GetRandomBech32ConsensusPubKey  = types.GetRandomBech32ConsensusPubKey
	GetRandomPubKey                 = types.GetRandomPubKey
	GetRandomPubKeySet              = types.GetRandomPubKeySet
	GetCurrentVersion               = types.GetCurrentVersion
	SetupConfigForTest              = types.SetupConfigForTest
	HasSimpleMajority               = types.HasSimpleMajority
	HasSuperMajority                = types.HasSuperMajority
	HasMinority                     = types.HasMinority
	DefaultGenesis                  = types.DefaultGenesis
	NewSolvencyVoter                = types.NewSolvencyVoter
	NewMsgSolvency                  = types.NewMsgSolvency
	NewMsgShielderRegisterPow       = types.NewMsgShielderRegisterPow
	NewMsgShielderPostCommitments   = types.NewMsgShielderPostCommitments
	NewMsgShielderRequestWithdrawal = types.NewMsgShielderRequestWithdrawal
	NewMsgShielderSplitFees         = types.NewMsgShielderSplitFees
	NewMsgNodeSlotAuctionCreate     = types.NewMsgNodeSlotAuctionCreate
	NewMsgNodeSlotAuctionBidPow     = types.NewMsgNodeSlotAuctionBidPow
	NewMsgNodeSlotAuctionSelectBid  = types.NewMsgNodeSlotAuctionSelectBid
	NewMsgNodeSlotAuctionSplit      = types.NewMsgNodeSlotAuctionSplit
)

type (
	// Msgs
	MsgSend                      = types.MsgSend
	MsgDeposit                   = types.MsgDeposit
	MsgNoOp                      = types.MsgNoOp
	MsgConsolidate               = types.MsgConsolidate
	MsgOutboundTx                = types.MsgOutboundTx
	MsgMimir                     = types.MsgMimir
	MsgNodePauseChain            = types.MsgNodePauseChain
	MsgMigrate                   = types.MsgMigrate
	MsgRefundTx                  = types.MsgRefundTx
	MsgErrataTx                  = types.MsgErrataTx
	MsgBan                       = types.MsgBan
	MsgSetVersion                = types.MsgSetVersion
	MsgProposeUpgrade            = types.MsgProposeUpgrade
	MsgApproveUpgrade            = types.MsgApproveUpgrade
	MsgRejectUpgrade             = types.MsgRejectUpgrade
	MsgSetIPAddress              = types.MsgSetIPAddress
	MsgSetNodeKeys               = types.MsgSetNodeKeys
	MsgMaint                     = types.MsgMaint
	MsgObservedTxIn              = types.MsgObservedTxIn
	MsgObservedTxOut             = types.MsgObservedTxOut
	MsgTssPool                   = types.MsgTssPool
	MsgTssKeysignFail            = types.MsgTssKeysignFail
	MsgNetworkFee                = types.MsgNetworkFee
	MsgSolvency                  = types.MsgSolvency
	MsgOperatorRotate            = types.MsgOperatorRotate
	MsgShielderRegisterPow       = types.MsgShielderRegisterPow
	MsgShielderPostCommitments   = types.MsgShielderPostCommitments
	MsgShielderRequestWithdrawal = types.MsgShielderRequestWithdrawal
	MsgNodeSlotAuctionCreate     = types.MsgNodeSlotAuctionCreate
	MsgNodeSlotAuctionBidPow     = types.MsgNodeSlotAuctionBidPow
	MsgNodeSlotAuctionSelectBid  = types.MsgNodeSlotAuctionSelectBid
	MsgNodeSlotAuctionSplit      = types.MsgNodeSlotAuctionSplit

	// Keeper structs
	ObservedTxs             = common.ObservedTxs
	ObservedTx              = common.ObservedTx
	ObservedTxVoter         = types.ObservedTxVoter
	ObservedTxVoters        = types.ObservedTxVoters
	BanVoter                = types.BanVoter
	ErrataTxVoter           = types.ErrataTxVoter
	TssVoter                = types.TssVoter
	TssKeysignFailVoter     = types.TssKeysignFailVoter
	TxOutItem               = types.TxOutItem
	TxOut                   = types.TxOut
	Keygen                  = types.Keygen
	KeygenBlock             = types.KeygenBlock
	EventSwap               = types.EventSwap
	EventDonate             = types.EventDonate
	EventRewards            = types.EventRewards
	EventErrata             = types.EventErrata
	PoolAmt                 = types.PoolAmt
	PoolMod                 = types.PoolMod
	PoolMods                = types.PoolMods
	Vault                   = types.Vault
	Vaults                  = types.Vaults
	NodeAccount             = types.NodeAccount
	NodeAccounts            = types.NodeAccounts
	NodeStatus              = types.NodeStatus
	Network                 = types.Network
	VaultStatus             = types.VaultStatus
	GasPool                 = types.GasPool
	EventGas                = types.EventGas
	EventPool               = types.EventPool
	EventRefund             = types.EventRefund
	EventBond               = types.EventBond
	EventFee                = types.EventFee
	EventSlash              = types.EventSlash
	EventOutbound           = types.EventOutbound
	NetworkFee              = types.NetworkFee
	PriceFeed               = types.PriceFeed
	OraclePrice             = types.OraclePrice
	ObservedNetworkFeeVoter = types.ObservedNetworkFeeVoter
	Jail                    = types.Jail
	ChainContract           = types.ChainContract
	Blame                   = types.Blame
	Node                    = types.Node
	NodeMimir               = types.NodeMimir
	NodeMimirs              = types.NodeMimirs
	ContractInfo            = types.ContractInfo
	ShielderSession         = types.ShielderSession
	ShielderDeposit         = types.ShielderDeposit
	ShielderWithdrawal      = types.ShielderWithdrawal
	NodeSlotAuction         = types.NodeSlotAuction
	NodeSlotBid             = types.NodeSlotBid

	// Proto
	ProtoStrings = types.ProtoStrings
	ProtoInt64   = types.ProtoInt64
)
