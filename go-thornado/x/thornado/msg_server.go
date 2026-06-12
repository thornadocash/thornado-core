package thornado

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	mgr Manager
}

var _ types.MsgServer = (*msgServer)(nil)

// NewMsgServerImpl returns an implementation of the module MsgServer interface.
func NewMsgServerImpl(mgr Manager) types.MsgServer {
	return &msgServer{mgr: mgr}
}

func (ms msgServer) Deposit(goCtx context.Context, msg *types.MsgDeposit) (*types.MsgEmpty, error) {
	handler := NewDepositHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ErrataTx(goCtx context.Context, msg *types.MsgErrataTx) (*types.MsgEmpty, error) {
	handler := NewErrataTxHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ErrataTxQuorum(goCtx context.Context, msg *types.MsgErrataTxQuorum) (*types.MsgEmpty, error) {
	handler := NewErrataTxQuorumHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) Config(goCtx context.Context, msg *types.MsgConfig) (*types.MsgEmpty, error) {
	handler := NewConfigHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) NetworkFee(goCtx context.Context, msg *types.MsgNetworkFee) (*types.MsgEmpty, error) {
	handler := NewNetworkFeeHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) NetworkFeeQuorum(goCtx context.Context, msg *types.MsgNetworkFeeQuorum) (*types.MsgEmpty, error) {
	handler := NewNetworkFeeQuorumHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) NodePauseChain(goCtx context.Context, msg *types.MsgNodePauseChain) (*types.MsgEmpty, error) {
	handler := NewNodePauseChainHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ObservedTxIn(goCtx context.Context, msg *types.MsgObservedTxIn) (*types.MsgEmpty, error) {
	handler := NewObservedTxInHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ObservedTxOut(goCtx context.Context, msg *types.MsgObservedTxOut) (*types.MsgEmpty, error) {
	handler := NewObservedTxOutHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ObservedTxQuorum(goCtx context.Context, msg *types.MsgObservedTxQuorum) (*types.MsgEmpty, error) {
	handler := NewObservedTxQuorumHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) DepositRequestPow(goCtx context.Context, msg *types.MsgDepositRequestPow) (*types.MsgDepositRequestPowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	owner, err := AccAddressFromCompressedSecp256k1(msg.DepositPubkey)
	if err != nil {
		return nil, err
	}
	session, err := RegisterDepositPowToken(ctx, ms.mgr.Keeper(), owner, msg.PowToken, msg.PowDurationMs)
	if err != nil {
		return nil, err
	}
	return &types.MsgDepositRequestPowResponse{
		DepositAddress:   session.DepositAddress.String(),
		VaultPubKey:      session.VaultPubKey.String(),
		DepositPathIndex: session.DepositPathIndex,
		ExpiresAtHeight:  session.ExpiresAtHeight,
		PurgeAtHeight:    session.PurgeAtHeight,
	}, nil
}

func (ms msgServer) ShielderShield(goCtx context.Context, msg *types.MsgShielderShield) (*types.MsgShielderShieldResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	owner, err := AccAddressFromCompressedSecp256k1(msg.DepositPubkey)
	if err != nil {
		return nil, err
	}
	session, err := ms.mgr.Keeper().GetDepositSession(ctx, owner)
	if err != nil {
		return nil, err
	}
	if session.DepositID.IsEmpty() {
		return nil, fmt.Errorf("deposit not matched")
	}
	depositID := session.DepositID
	deposit, err := ms.mgr.Keeper().GetDepositRecord(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if deposit.DepositID.IsEmpty() {
		return nil, fmt.Errorf("deposit not found")
	}
	noteCommitments, err := parseShielderNoteCommitments(msg.Commitments, deposit.AmountSats, deposit.IsNodeBond())
	if err != nil {
		return nil, err
	}
	amountSats := shielderNoteCommitmentTotal(noteCommitments)
	if amountSats == 0 {
		return nil, fmt.Errorf("missing shielder commitment amount")
	}
	shieldRef := strings.TrimSpace(msg.DepositPubkey)
	if err := VerifyShieldAuthorization(msg.DepositPubkey, msg.Signature, shieldRef, amountSats, msg.Commitments); err != nil {
		return nil, err
	}
	deposit, err = PostShielderShield(ctx, ms.mgr.Keeper(), owner, depositID, msg.Commitments)
	if err != nil {
		return nil, err
	}
	return &types.MsgShielderShieldResponse{
		Status: deposit.Status,
	}, nil
}

func (ms msgServer) ShielderRedeem(goCtx context.Context, msg *types.MsgShielderRedeem) (*types.MsgShielderRedeemResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	authorization, err := AuthorizeShielderRedeem(ctx, ms.mgr.Keeper(), ShielderRedeemRequest{
		Proof:  msg.Proof,
		Public: msg.Public,
	})
	if err != nil {
		return nil, err
	}
	withdrawal, err := FinalizeShielderRedeem(ctx, ms.mgr.Keeper(), authorization)
	if err != nil {
		return nil, err
	}
	return &types.MsgShielderRedeemResponse{
		WithdrawalId: withdrawal.WithdrawalID,
		Status:       withdrawal.Status,
	}, nil
}

func (ms msgServer) BondFromNotes(goCtx context.Context, msg *types.MsgBondFromNotes) (*types.MsgBondFromNotesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	bond, err := BondFromShieldedNotes(ctx, ms.mgr.Keeper(), msg.Signer, msg.NodePubKey, msg.OperatorPubKey, ShielderRedeemRequest{
		Proof:  msg.Proof,
		Public: msg.Public,
	})
	if err != nil {
		return nil, err
	}
	status := NodeStandby.String()
	if bond.BondSats == 0 {
		status = "pending"
	}
	return &types.MsgBondFromNotesResponse{
		BondSats: bond.BondSats,
		Slot:     bond.Slot,
		Status:   status,
	}, nil
}

func (ms msgServer) ShielderShieldFees(goCtx context.Context, msg *types.MsgShielderShieldFees) (*types.MsgShielderShieldFeesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	deposit, err := ShieldShielderFees(ctx, ms.mgr.Keeper(), msg.Signer, msg.NodePubKey, msg.OperatorSignature, msg.Commitments, msg.FeeNotePubKeys)
	if err != nil {
		return nil, err
	}
	return &types.MsgShielderShieldFeesResponse{
		DepositId:  deposit.DepositID.String(),
		AmountSats: deposit.AmountSats,
		Status:     deposit.Status,
	}, nil
}

func (ms msgServer) NodeSlotAuctionCreate(goCtx context.Context, msg *types.MsgNodeSlotAuctionCreate) (*types.MsgNodeSlotAuctionCreateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	auction, err := CreateNodeSlotAuction(ctx, ms.mgr.Keeper(), msg.Signer, msg.NodePubKey, msg.ReserveSats, msg.ExpiryHeight)
	if err != nil {
		return nil, err
	}
	return &types.MsgNodeSlotAuctionCreateResponse{AuctionId: auction.AuctionID}, nil
}

func (ms msgServer) NodeSlotAuctionBidCreate(goCtx context.Context, msg *types.MsgNodeSlotAuctionBidCreate) (*types.MsgNodeSlotAuctionBidCreateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	bid, err := CreateNodeSlotBid(ctx, ms.mgr.Keeper(), msg.Signer, msg.AuctionId, msg.OperatorPubKey, msg.NodePubKey)
	if err != nil {
		return nil, err
	}
	return &types.MsgNodeSlotAuctionBidCreateResponse{
		BidId: bid.BidID,
	}, nil
}

func (ms msgServer) NodeSlotAuctionSelectBid(goCtx context.Context, msg *types.MsgNodeSlotAuctionSelectBid) (*types.MsgEmpty, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	if _, _, err := SelectNodeSlotBid(ctx, ms.mgr.Keeper(), msg.Signer, msg.AuctionId, msg.BidId); err != nil {
		return nil, err
	}
	return &types.MsgEmpty{}, nil
}

func (ms msgServer) NodeSaleShield(goCtx context.Context, msg *types.MsgNodeSaleShield) (*types.MsgShielderShieldResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	deposit, err := ShieldNodeSlotSaleEntitlement(ctx, ms.mgr.Keeper(), msg.Signer, msg.AuctionId, msg.BidId, msg.DepositPubkey, msg.Signature, msg.Commitments)
	if err != nil {
		return nil, err
	}
	return &types.MsgShielderShieldResponse{Status: deposit.Status}, nil
}

func (ms msgServer) Send(goCtx context.Context, msg *types.MsgSend) (*types.MsgEmpty, error) {
	handler := NewSendHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) SetIPAddress(goCtx context.Context, msg *types.MsgSetIPAddress) (*types.MsgEmpty, error) {
	handler := NewIPAddressHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) SetNodeKeys(goCtx context.Context, msg *types.MsgSetNodeKeys) (*types.MsgEmpty, error) {
	handler := NewSetNodeKeysHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) Solvency(goCtx context.Context, msg *types.MsgSolvency) (*types.MsgEmpty, error) {
	handler := NewSolvencyHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) SolvencyQuorum(goCtx context.Context, msg *types.MsgSolvencyQuorum) (*types.MsgEmpty, error) {
	handler := NewSolvencyQuorumHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) FrostKeysignFail(goCtx context.Context, msg *types.MsgFrostKeysignFail) (*types.MsgEmpty, error) {
	handler := NewFrostKeysignHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) KeygenVault(goCtx context.Context, msg *types.MsgKeygenVault) (*types.MsgEmpty, error) {
	handler := NewFrostHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) SetVersion(goCtx context.Context, msg *types.MsgSetVersion) (*types.MsgEmpty, error) {
	handler := NewVersionHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ProposeUpgrade(goCtx context.Context, msg *types.MsgProposeUpgrade) (*types.MsgEmpty, error) {
	handler := NewProposeUpgradeHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) ApproveUpgrade(goCtx context.Context, msg *types.MsgApproveUpgrade) (*types.MsgEmpty, error) {
	handler := NewApproveUpgradeHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func (ms msgServer) RejectUpgrade(goCtx context.Context, msg *types.MsgRejectUpgrade) (*types.MsgEmpty, error) {
	handler := NewRejectUpgradeHandler(ms.mgr)
	return externalHandler(goCtx, handler, msg)
}

func externalHandler(goCtx context.Context, handler MsgHandler, msg sdk.Msg) (_ *types.MsgEmpty, err error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx = ctx.WithLogger(ctx.Logger().With("height", ctx.BlockHeight()))

	defer func() {
		if r := recover(); r != nil {
			// print stack
			stack := make([]byte, 1024)
			length := runtime.Stack(stack, true)
			ctx.Logger().Error("panic", "msg", msg)
			fmt.Println(string(stack[:length]))
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	result, err := handler.Run(ctx, msg)

	if result != nil && result.Size() > 0 {
		return nil, fmt.Errorf("external handler, handler returned non-empty result, %s", msg)
	}
	if err != nil {
		if _, code, _ := errorsmod.ABCIInfo(err, false); code == 1 {
			// This would be redacted, so wrap it.
			err = errorsmod.Wrap(errInternal, err.Error())
		}
		return nil, err
	}

	return &types.MsgEmpty{}, err
}
