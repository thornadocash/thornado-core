package types

import (
	"cosmossdk.io/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"google.golang.org/protobuf/reflect/protoreflect"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

var ModuleCdc = codec.NewLegacyAmino()

func init() {
	RegisterLegacyAminoCodec(ModuleCdc)
}

// RegisterCodec register the msg types for amino
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgKeygenVault{}, ModuleName+"/KeygenVault", nil)
	cdc.RegisterConcrete(&MsgTssKeysignFail{}, ModuleName+"/TssKeysignFail", nil)
	cdc.RegisterConcrete(&MsgObservedTxIn{}, ModuleName+"/ObservedTxIn", nil)
	cdc.RegisterConcrete(&MsgObservedTxQuorum{}, ModuleName+"/ObservedTxQuorum", nil)
	cdc.RegisterConcrete(&MsgObservedTxOut{}, ModuleName+"/ObservedTxOut", nil)
	cdc.RegisterConcrete(&MsgNoOp{}, ModuleName+"/MsgNoOp", nil)
	cdc.RegisterConcrete(&MsgOutboundTx{}, ModuleName+"/MsgOutboundTx", nil)
	cdc.RegisterConcrete(&MsgSetVersion{}, ModuleName+"/MsgSetVersion", nil)
	cdc.RegisterConcrete(&MsgProposeUpgrade{}, ModuleName+"/MsgProposeUpgrade", nil)
	cdc.RegisterConcrete(&MsgApproveUpgrade{}, ModuleName+"/MsgApproveUpgrade", nil)
	cdc.RegisterConcrete(&MsgRejectUpgrade{}, ModuleName+"/MsgRejectUpgrade", nil)
	cdc.RegisterConcrete(&MsgSetNodeKeys{}, ModuleName+"/MsgSetNodeKeys", nil)
	cdc.RegisterConcrete(&MsgSetIPAddress{}, ModuleName+"/MsgSetIPAddress", nil)
	cdc.RegisterConcrete(&MsgErrataTx{}, ModuleName+"/MsgErrataTx", nil)
	cdc.RegisterConcrete(&MsgErrataTxQuorum{}, ModuleName+"/MsgErrataTxQuorum", nil)
	cdc.RegisterConcrete(&MsgConfig{}, ModuleName+"/MsgConfig", nil)
	cdc.RegisterConcrete(&MsgDeposit{}, ModuleName+"/MsgDeposit", nil)
	cdc.RegisterConcrete(&MsgNetworkFee{}, ModuleName+"/MsgNetworkFee", nil)
	cdc.RegisterConcrete(&MsgNetworkFeeQuorum{}, ModuleName+"/MsgNetworkFeeQuorum", nil)
	cdc.RegisterConcrete(&MsgMigrate{}, ModuleName+"/MsgMigrate", nil)
	cdc.RegisterConcrete(&MsgSend{}, ModuleName+"/MsgSend", nil)
	cdc.RegisterConcrete(&MsgNodePauseChain{}, ModuleName+"/MsgNodePauseChain", nil)
	cdc.RegisterConcrete(&MsgSolvency{}, ModuleName+"/MsgSolvency", nil)
	cdc.RegisterConcrete(&MsgSolvencyQuorum{}, ModuleName+"/MsgSolvencyQuorum", nil)
	cdc.RegisterConcrete(&MsgPriceFeedQuorumBatch{}, ModuleName+"/MsgPriceFeedQuorumBatch", nil)
	cdc.RegisterConcrete(&MsgDepositRequestPow{}, ModuleName+"/MsgDepositRequestPow", nil)
	cdc.RegisterConcrete(&MsgShielderShield{}, ModuleName+"/MsgShielderShield", nil)
	cdc.RegisterConcrete(&MsgShielderRedeem{}, ModuleName+"/MsgShielderRedeem", nil)
	cdc.RegisterConcrete(&MsgShielderShieldFees{}, ModuleName+"/MsgShielderShieldFees", nil)
	cdc.RegisterConcrete(&MsgNodeSlotAuctionCreate{}, ModuleName+"/MsgNodeSlotAuctionCreate", nil)
	cdc.RegisterConcrete(&MsgNodeSlotAuctionBidPow{}, ModuleName+"/MsgNodeSlotAuctionBidPow", nil)
	cdc.RegisterConcrete(&MsgNodeSlotAuctionSelectBid{}, ModuleName+"/MsgNodeSlotAuctionSelectBid", nil)
	cdc.RegisterConcrete(&MsgNodeSlotAuctionShield{}, ModuleName+"/MsgNodeSlotAuctionShield", nil)
	cdc.RegisterConcrete(&MsgBondFromNotes{}, ModuleName+"/MsgBondFromNotes", nil)
}

// RegisterInterfaces register the types
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgKeygenVault{},
		&MsgTssKeysignFail{},
		&MsgObservedTxIn{},
		&MsgObservedTxOut{},
		&MsgObservedTxQuorum{},
		&MsgNoOp{},
		&MsgOutboundTx{},
		&MsgSetVersion{},
		&MsgProposeUpgrade{},
		&MsgApproveUpgrade{},
		&MsgRejectUpgrade{},
		&MsgSetNodeKeys{},
		&MsgSetIPAddress{},
		&MsgErrataTx{},
		&MsgErrataTxQuorum{},
		&MsgConfig{},
		&MsgDeposit{},
		&MsgNetworkFee{},
		&MsgNetworkFeeQuorum{},
		&MsgMigrate{},
		&MsgSend{},
		&MsgNodePauseChain{},
		&MsgSolvency{},
		&MsgSolvencyQuorum{},
		&MsgPriceFeedQuorumBatch{},
		&MsgDepositRequestPow{},
		&MsgShielderShield{},
		&MsgShielderRedeem{},
		&MsgShielderShieldFees{},
		&MsgNodeSlotAuctionCreate{},
		&MsgNodeSlotAuctionBidPow{},
		&MsgNodeSlotAuctionSelectBid{},
		&MsgNodeSlotAuctionShield{},
		&MsgBondFromNotes{},
	)

	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

func DefineCustomGetSigners(signingOptions *signing.Options) {
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgDeposit"), MsgDepositCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgErrataTx"), MsgErrataCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgErrataTxQuorum"), MsgErrataTxQuorumCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgConfig"), MsgConfigCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNetworkFee"), MsgNetworkFeeCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNetworkFeeQuorum"), MsgNetworkFeeQuorumCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNodePauseChain"), MsgNodePauseChainCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgObservedTxIn"), MsgObservedTxInCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgObservedTxQuorum"), MsgObservedTxQuorumCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgObservedTxOut"), MsgObservedTxOutCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSend"), MsgSendCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSetIPAddress"), MsgSetIPAddressCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSetNodeKeys"), MsgSetNodeKeysCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSolvency"), MsgSolvencyCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSolvencyQuorum"), MsgSolvencyQuorumCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgTssKeysignFail"), MsgTssKeysignFailCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgKeygenVault"), MsgKeygenVaultCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgSetVersion"), MsgSetVersionCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgProposeUpgrade"), MsgProposeUpgradeCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgApproveUpgrade"), MsgApproveUpgradeCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgRejectUpgrade"), MsgRejectUpgradeCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgPriceFeedQuorumBatch"), MsgPriceFeedQuorumBatchCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgDepositRequestPow"), MsgDepositRequestPowCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgShielderShield"), MsgShielderShieldCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgShielderRedeem"), MsgShielderRedeemCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgShielderShieldFees"), MsgShielderShieldFeesCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNodeSlotAuctionCreate"), MsgNodeSlotAuctionCreateCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNodeSlotAuctionBidPow"), MsgNodeSlotAuctionBidPowCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNodeSlotAuctionSelectBid"), MsgNodeSlotAuctionSelectBidCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgNodeSlotAuctionShield"), MsgNodeSlotAuctionShieldCustomGetSigners)
	signingOptions.DefineCustomGetSigners(protoreflect.FullName("types.MsgBondFromNotes"), MsgBondFromNotesCustomGetSigners)
}
