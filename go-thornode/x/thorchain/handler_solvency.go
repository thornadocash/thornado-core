package thorchain

import (
	"fmt"

	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

// SolvencyHandler is to process MsgSolvency message from bifrost
// Bifrost constantly monitor the account balance , and report to THORNode
// If it detect that wallet is short of fund , much less than vault, the network should automatically halt trading
type SolvencyHandler struct {
	mgr Manager
}

// NewSolvencyHandler create a new instance of solvency handler
func NewSolvencyHandler(mgr Manager) SolvencyHandler {
	return SolvencyHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to process MsgSolvency
func (h SolvencyHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgSolvency)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg solvency failed validation", "error", err)
		return nil, err
	}
	return h.handle(ctx, *msg)
}

func (h SolvencyHandler) validate(ctx cosmos.Context, msg MsgSolvency) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(fmt.Sprintf("%+v are not authorized", msg.GetSigners()))
	}
	return nil
}

// handleCurrent is the logic to process MsgSolvency, the feature works like this
//  1. Bifrost report MsgSolvency to thornode , which is the balance of asgard wallet on each individual chain
//  2. once MsgSolvency reach consensus , then the network compare the wallet balance against wallet
//     if wallet has less fund than asgard vault , and the gap is more than 1% , then the chain
//     that is insolvent will be halt
//  3. When chain is halt , bifrost will not observe inbound , and will not sign outbound txs until the issue has been investigated , and enabled it again using mimir
func (h SolvencyHandler) handle(ctx cosmos.Context, msg MsgSolvency) (*cosmos.Result, error) {
	ctx.Logger().Debug("handle Solvency request", "id", msg.Id.String(), "signer", msg.Signer.String())

	k := h.mgr.Keeper()

	active, err := k.ListActiveValidators(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get list of active node accounts")
	}

	voter, err := k.GetSolvencyVoter(ctx, msg.Id, msg.Chain)
	if err != nil {
		return &cosmos.Result{}, fmt.Errorf("fail to get solvency voter, err: %w", err)
	}
	if voter.Empty() {
		voter = NewSolvencyVoter(msg.Id, msg.Chain, msg.PubKey, msg.Coins, msg.Height)
	}

	defer func() {
		k.SetSolvencyVoter(ctx, voter)
	}()

	s := &common.Solvency{
		Id:     msg.Id,
		Chain:  msg.Chain,
		PubKey: msg.PubKey,
		Coins:  msg.Coins,
		Height: msg.Height,
	}

	if err := processSolvencyAttestation(ctx, h.mgr, &voter, msg.Signer, active, s, true); err != nil {
		ctx.Logger().Error("fail to process solvency attestation", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

// SolvencyAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func SolvencyAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgSolvency) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
