package keeperv1

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	ModuleName  = types.ModuleName
	ReserveName = types.ReserveName
	BaseName    = types.BaseName
	BondName    = types.BondName
	StoreKey    = types.StoreKey

	// Vaults
	BaseVault     = types.VaultType_BaseVault
	ActiveVault   = types.VaultStatus_ActiveVault
	InitVault     = types.VaultStatus_InitVault
	RetiringVault = types.VaultStatus_RetiringVault
	InactiveVault = types.VaultStatus_InactiveVault

	// Node status
	NodeActive  = types.NodeStatus_Active
	NodeStandby = types.NodeStatus_Standby
	NodeUnknown = types.NodeStatus_Unknown

	// Node type
	NodeTypeUnknown = types.NodeType_TypeUnknown
	NodeTypeNode    = types.NodeType_TypeNode
	NodeTypeVault   = types.NodeType_TypeVault

	// Mint/Burn type
	MintSupplyType = types.MintBurnSupplyType_mint
	BurnSupplyType = types.MintBurnSupplyType_burn

	// Bond type
	BaseVaultKeygen = types.KeygenType_BaseVaultKeygen
	BondReturned    = types.BondType_bond_returned

	TxOutStatusPendingBatch = types.TxOutStatusPendingBatch
	TxOutStatusPendingSign  = types.TxOutStatusPendingSign
	TxOutStatusPendingRetry = types.TxOutStatusPendingRetry
)

var (
	NewJail                    = types.NewJail
	NewNetwork                 = types.NewNetwork
	NewObservedTx              = common.NewObservedTx
	NewTssVoter                = types.NewTssVoter
	NewErrataTxVoter           = types.NewErrataTxVoter
	NewObservedTxVoter         = types.NewObservedTxVoter
	NewKeygen                  = types.NewKeygen
	NewKeygenBlock             = types.NewKeygenBlock
	NewTxOut                   = types.NewTxOut
	HasSuperMajority           = types.HasSuperMajority
	RegisterLegacyAminoCodec   = types.RegisterLegacyAminoCodec
	NewNodeAccount             = types.NewNodeAccount
	NewVault                   = types.NewVault
	NewVaultV2                 = types.NewVaultV2
	NewEventBond               = types.NewEventBond
	NewEventMintBurn           = types.NewEventMintBurn
	GetRandomTx                = types.GetRandomTx
	GetRandomNode              = types.GetRandomNode
	GetRandomVaultNode         = types.GetRandomVaultNode
	GetRandomBTCAddress        = types.GetRandomBTCAddress
	GetRandomThornadoAddress   = types.GetRandomThornadoAddress
	GetRandomTxHash            = types.GetRandomTxHash
	GetRandomBech32Addr        = types.GetRandomBech32Addr
	GetRandomPubKey            = types.GetRandomPubKey
	GetRandomPubKeySet         = types.GetRandomPubKeySet
	GetCurrentVersion          = types.GetCurrentVersion
	NewObservedNetworkFeeVoter = types.NewObservedNetworkFeeVoter
	NewNetworkFee              = types.NewNetworkFee
	NewTssKeysignFailVoter     = types.NewTssKeysignFailVoter
	SetupConfigForTest         = types.SetupConfigForTest
)

type (
	ObservedTxs             = common.ObservedTxs
	ObservedTxVoter         = types.ObservedTxVoter
	ErrataTxVoter           = types.ErrataTxVoter
	TssVoter                = types.TssVoter
	TssKeysignFailVoter     = types.TssKeysignFailVoter
	TxOutItem               = types.TxOutItem
	TxOut                   = types.TxOut
	KeygenBlock             = types.KeygenBlock
	Vault                   = types.Vault
	Vaults                  = types.Vaults
	Jail                    = types.Jail
	NodeAccount             = types.NodeAccount
	NodeAccounts            = types.NodeAccounts
	NodeStatus              = types.NodeStatus
	NodeType                = types.NodeType
	Network                 = types.Network
	VaultStatus             = types.VaultStatus
	NetworkFee              = types.NetworkFee
	ObservedNetworkFeeVoter = types.ObservedNetworkFeeVoter
	TssKeygenMetric         = types.TssKeygenMetric
	TssKeysignMetric        = types.TssKeysignMetric
	SolvencyVoter           = types.SolvencyVoter
	MinJoinLast             = types.MinJoinLast
	NodeConfig              = types.NodeConfig
	NodeConfigs             = types.NodeConfigs
	OraclePrice             = types.OraclePrice
	PriceFeed               = types.PriceFeed

	ProtoInt64        = types.ProtoInt64
	ProtoUint64       = types.ProtoUint64
	ProtoAccAddress   = types.ProtoAccAddress
	ProtoAccAddresses = types.ProtoAccAddresses
	ProtoString       = types.ProtoString
	ProtoStrings      = types.ProtoStrings
	ProtoUint         = common.ProtoUint
	ProtoBools        = types.ProtoBools
)
