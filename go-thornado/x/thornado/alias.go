package thornado

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	ModuleName       = types.ModuleName
	ReserveName      = types.ReserveName
	BaseName         = types.BaseName
	BondName         = types.BondName
	RouterKey        = types.RouterKey
	StoreKey         = types.StoreKey
	DefaultCodespace = types.DefaultCodespace

	// Vaults
	BaseVault     = types.VaultType_BaseVault
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
	BondPaid        = types.BondType_bond_paid
	BondReturned    = types.BondType_bond_returned
	BaseVaultKeygen = types.KeygenType_BaseVaultKeygen

	TxOutStatusPendingBatch = types.TxOutStatusPendingBatch
	TxOutStatusPendingSign  = types.TxOutStatusPendingSign
	TxOutStatusPendingRetry = types.TxOutStatusPendingRetry

	// Mint/Burn type
	MintSupplyType = types.MintBurnSupplyType_mint
	BurnSupplyType = types.MintBurnSupplyType_burn
)

var (
	NewNetwork                     = types.NewNetwork
	NewObservedTx                  = common.NewObservedTx
	NewFrostVoter                    = types.NewFrostVoter
	NewErrataTxVoter               = types.NewErrataTxVoter
	NewObservedTxVoter             = types.NewObservedTxVoter
	NewMsgConfig                   = types.NewMsgConfig
	NewMsgNodePauseChain           = types.NewMsgNodePauseChain
	NewMsgDeposit                  = types.NewMsgDeposit
	NewMsgKeygenVault              = types.NewMsgKeygenVault
	NewMsgKeygenVaultV2            = types.NewMsgKeygenVaultV2
	NewMsgFrostKeysignFail           = types.NewMsgFrostKeysignFail
	NewMsgObservedTxIn             = types.NewMsgObservedTxIn
	NewMsgObservedTxOut            = types.NewMsgObservedTxOut
	NewMsgNoOp                     = types.NewMsgNoOp
	NewMsgConsolidate              = types.NewMsgConsolidate
	NewKeygen                      = types.NewKeygen
	NewKeygenBlock                 = types.NewKeygenBlock
	NewMsgSetNodeKeys              = types.NewMsgSetNodeKeys
	NewMsgOperatorRotate           = types.NewMsgOperatorRotate
	NewMsgPriceFeedQuorumBatch     = types.NewMsgPriceFeedQuorumBatch
	NewTxOut                       = types.NewTxOut
	NewEventBond                   = types.NewEventBond
	NewEventGas                    = types.NewEventGas
	NewEventScheduledOutbound      = types.NewEventScheduledOutbound
	NewEventSecurity               = types.NewEventSecurity
	NewEventPenaltyPoint           = types.NewEventPenaltyPoint
	NewEventFee                    = types.NewEventFee
	NewEventOutbound               = types.NewEventOutbound
	NewEventSetConfig              = types.NewEventSetConfig
	NewEventSetNodeConfig          = types.NewEventSetNodeConfig
	NewEventFrostKeygenSuccess       = types.NewEventFrostKeygenSuccess
	NewEventFrostKeygenFailure       = types.NewEventFrostKeygenFailure
	NewEventFrostKeygenMetric        = types.NewEventFrostKeygenMetric
	NewEventFrostKeysignMetric       = types.NewEventFrostKeysignMetric
	NewEventMintBurn               = types.NewEventMintBurn
	NewEventVersion                = types.NewEventVersion
	NewEventOperatorRotate         = types.NewEventOperatorRotate
	NewEventOraclePrice            = types.NewEventOraclePrice
	NewEventFailedOutboundRecovery = types.NewEventFailedOutboundRecovery
	NewMsgOutboundTx               = types.NewMsgOutboundTx
	NewMsgMigrate                  = types.NewMsgMigrate
	ModuleCdc                      = types.ModuleCdc
	RegisterLegacyAminoCodec       = types.RegisterLegacyAminoCodec
	RegisterInterfaces             = types.RegisterInterfaces
	NewNodeAccount                 = types.NewNodeAccount
	NewVault                       = types.NewVault
	NewVaultV2                     = types.NewVaultV2
	NewMsgErrataTx                 = types.NewMsgErrataTx
	NewMsgSetVersion               = types.NewMsgSetVersion
	NewMsgProposeUpgrade           = types.NewMsgProposeUpgrade
	NewMsgApproveUpgrade           = types.NewMsgApproveUpgrade
	NewMsgRejectUpgrade            = types.NewMsgRejectUpgrade
	NewMsgSetIPAddress             = types.NewMsgSetIPAddress
	NewMsgNetworkFee               = types.NewMsgNetworkFee
	NewNetworkFee                  = types.NewNetworkFee
	GetRandomVault                 = types.GetRandomVault
	GetRandomTx                    = types.GetRandomTx
	GetRandomObservedTx            = types.GetRandomObservedTx
	GetRandomTxOutItem             = types.GetRandomTxOutItem
	GetRandomObservedTxVoter       = types.GetRandomObservedTxVoter
	GetRandomNode                  = types.GetRandomNode
	GetRandomVaultNode             = types.GetRandomVaultNode
	GetRandomThornadoAddress       = types.GetRandomThornadoAddress
	GetRandomBTCAddress            = types.GetRandomBTCAddress
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
	NewMsgDepositRequestPow        = types.NewMsgDepositRequestPow
	NewMsgShielderShield           = types.NewMsgShielderShield
	NewMsgShielderRedeem           = types.NewMsgShielderRedeem
	NewMsgShielderShieldFees       = types.NewMsgShielderShieldFees
	NewMsgNodeSlotAuctionCreate    = types.NewMsgNodeSlotAuctionCreate
	NewMsgNodeSlotAuctionBidCreate = types.NewMsgNodeSlotAuctionBidCreate
	NewMsgNodeSlotAuctionSelectBid = types.NewMsgNodeSlotAuctionSelectBid
	NewMsgNodeSaleShield           = types.NewMsgNodeSaleShield
	NewMsgBondFromNotes            = types.NewMsgBondFromNotes
)

type (
	// Msgs
	MsgSend                     = types.MsgSend
	MsgDeposit                  = types.MsgDeposit
	MsgNoOp                     = types.MsgNoOp
	MsgConsolidate              = types.MsgConsolidate
	MsgOutboundTx               = types.MsgOutboundTx
	MsgConfig                   = types.MsgConfig
	MsgNodePauseChain           = types.MsgNodePauseChain
	MsgMigrate                  = types.MsgMigrate
	MsgErrataTx                 = types.MsgErrataTx
	MsgSetVersion               = types.MsgSetVersion
	MsgProposeUpgrade           = types.MsgProposeUpgrade
	MsgApproveUpgrade           = types.MsgApproveUpgrade
	MsgRejectUpgrade            = types.MsgRejectUpgrade
	MsgSetIPAddress             = types.MsgSetIPAddress
	MsgSetNodeKeys              = types.MsgSetNodeKeys
	MsgMaint                    = types.MsgMaint
	MsgObservedTxIn             = types.MsgObservedTxIn
	MsgObservedTxOut            = types.MsgObservedTxOut
	MsgKeygenVault              = types.MsgKeygenVault
	MsgFrostKeysignFail           = types.MsgFrostKeysignFail
	MsgNetworkFee               = types.MsgNetworkFee
	MsgSolvency                 = types.MsgSolvency
	MsgOperatorRotate           = types.MsgOperatorRotate
	MsgDepositRequestPow        = types.MsgDepositRequestPow
	MsgShielderShield           = types.MsgShielderShield
	MsgShielderRedeem           = types.MsgShielderRedeem
	MsgNodeSlotAuctionCreate    = types.MsgNodeSlotAuctionCreate
	MsgNodeSlotAuctionBidCreate = types.MsgNodeSlotAuctionBidCreate
	MsgNodeSlotAuctionSelectBid = types.MsgNodeSlotAuctionSelectBid
	MsgNodeSaleShield           = types.MsgNodeSaleShield
	MsgBondFromNotes            = types.MsgBondFromNotes

	// Keeper structs
	ObservedTxs             = common.ObservedTxs
	ObservedTx              = common.ObservedTx
	ObservedTxVoter         = types.ObservedTxVoter
	ObservedTxVoters        = types.ObservedTxVoters
	ErrataTxVoter           = types.ErrataTxVoter
	FrostVoter                = types.FrostVoter
	FrostKeysignFailVoter     = types.FrostKeysignFailVoter
	TxOutItem               = types.TxOutItem
	TxOut                   = types.TxOut
	Keygen                  = types.Keygen
	KeygenBlock             = types.KeygenBlock
	Vault                   = types.Vault
	Vaults                  = types.Vaults
	NodeAccount             = types.NodeAccount
	NodeAccounts            = types.NodeAccounts
	NodeStatus              = types.NodeStatus
	Network                 = types.Network
	VaultStatus             = types.VaultStatus
	GasPool                 = types.GasPool
	EventGas                = types.EventGas
	EventBond               = types.EventBond
	EventFee                = types.EventFee
	EventOutbound           = types.EventOutbound
	NetworkFee              = types.NetworkFee
	PriceFeed               = types.PriceFeed
	OraclePrice             = types.OraclePrice
	ObservedNetworkFeeVoter = types.ObservedNetworkFeeVoter
	Jail                    = types.Jail
	Blame                   = types.Blame
	Node                    = types.Node
	NodeConfig              = types.NodeConfig
	NodeConfigs             = types.NodeConfigs
	DepositSession          = types.DepositSession
	DepositRecord           = types.DepositRecord
	ShielderRedeem          = types.ShielderRedeem
	NodeSlotAuction         = types.NodeSlotAuction
	NodeSlotBid             = types.NodeSlotBid

	// Proto
	ProtoStrings = types.ProtoStrings
	ProtoInt64   = types.ProtoInt64
)
