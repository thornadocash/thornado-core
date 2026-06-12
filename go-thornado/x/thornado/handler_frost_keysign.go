package thornado

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"
	frostMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// FrostKeysignHandler is design to process MsgFrostKeysignFail
type FrostKeysignHandler struct {
	mgr Manager
}

// NewFrostKeysignHandler create a new instance of FrostKeysignHandler
// when a signer fail to join frost keysign , thornado need to penalty the node account
func NewFrostKeysignHandler(mgr Manager) FrostKeysignHandler {
	return FrostKeysignHandler{
		mgr: mgr,
	}
}

// Run is the main entry to process MsgFrostKeysignFail
func (h FrostKeysignHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgFrostKeysignFail)
	if !ok {
		return nil, errInvalidMessage
	}
	err := h.validate(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("MsgFrostKeysignFail failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("failed to process MsgFrostKeysignFail", "error", err)
	}
	return result, err
}

func (h FrostKeysignHandler) validate(ctx cosmos.Context, msg MsgFrostKeysignFail) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	m, err := NewMsgFrostKeysignFail(msg.Height, msg.Blame, msg.Coins, msg.Signer, msg.PubKey)
	if err != nil {
		ctx.Logger().Error("fail to reconstruct keysign fail msg", "error", err)
		return err
	}
	if !strings.EqualFold(m.ID, msg.ID) {
		return cosmos.ErrUnknownRequest("invalid keysign fail message")
	}

	if _, err = validateKeysignAuth(ctx, h.mgr.Keeper(), msg.GetSigners()); err != nil {
		return err
	}

	active, err := h.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return wrapError(ctx, err, "fail to get list of active node accounts")
	}

	if !HasSimpleMajority(len(active)-len(msg.Blame.BlameNodes), len(active)) {
		ctx.Logger().Error("blame cast too wide", "blame", len(msg.Blame.BlameNodes))
		return fmt.Errorf("blame cast too wide: %d/%d", len(msg.Blame.BlameNodes), len(active))
	}

	return nil
}

func (h FrostKeysignHandler) handle(ctx cosmos.Context, msg MsgFrostKeysignFail) (*cosmos.Result, error) {
	ctx.Logger().Info("handle MsgFrostKeysignFail request", "ID", msg.ID, "signer", msg.Signer, "pubkey", msg.PubKey, "blame", msg.Blame.String())
	voter, err := h.mgr.Keeper().GetFrostKeysignFailVoter(ctx, msg.ID)
	if err != nil {
		return nil, err
	}
	observePenaltyPoints := h.mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)

	// add labels to telemetry context
	labels := []metrics.Label{
		telemetry.NewLabel("reason", "failed_keysign"),
	}
	if len(msg.Coins) == 1 { // only label when penalty is for single asset
		labels = append(
			labels,
			telemetry.NewLabel("chain", string(msg.Coins[0].Asset.Chain)),
			telemetry.NewLabel("symbol", string(msg.Coins[0].Asset.Symbol)),
		)
	}
	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, labels))

	h.mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, msg.Signer)
	if !voter.Sign(msg.Signer) {
		ctx.Logger().Info("signer already signed MsgFrostKeysignFail", "signer", msg.Signer.String(), "txid", msg.ID)
		return &cosmos.Result{}, nil
	}

	vault, err := h.mgr.Keeper().GetVault(ctx, msg.PubKey)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get vault")
	}
	if vault.IsEmpty() {
		h.mgr.Keeper().SetFrostKeysignFailVoter(ctx, voter)
		return &cosmos.Result{}, nil
	}
	var vaultMemberNodes NodeAccounts
	for _, item := range vault.GetMembership() {
		var addr cosmos.AccAddress
		addr, err = item.GetThorAddress()
		if err != nil {
			return nil, wrapError(ctx, err, "fail to get thor address for "+item.String())
		}
		var na NodeAccount
		na, err = h.mgr.Keeper().GetNodeAccount(ctx, addr)
		if err != nil {
			return nil, wrapError(ctx, err, "fail to get node account")
		}
		vaultMemberNodes = append(vaultMemberNodes, na)
	}

	// Track terminal-round failures from the vault's actual signing committee,
	// not just the current active node set. Retiring vault members can
	// still participate in keysign during churn after leaving the active set,
	// so using IsNodeKeys here would undercount final-round reports and could
	// miss freezing a vault that is still failing at the terminal round.
	if msg.Blame.Round == frostMessages.KEYSIGN7 || msg.Blame.Round == frostMessages.EDDSAKEYSIGN3 {
		for _, na := range vaultMemberNodes {
			if msg.Signer.Equals(na.NodeAddress) {
				voter.LastRoundCount++
				break
			}
		}
	}
	h.mgr.Keeper().SetFrostKeysignFailVoter(ctx, voter)

	// doesn't have consensus yet
	if !voter.HasConsensus(vaultMemberNodes) {
		return &cosmos.Result{}, nil
	}
	violaters := make([]string, len(msg.Blame.BlameNodes))
	for i, node := range msg.Blame.BlameNodes {
		violaters[i] = node.Pubkey
	}
	ctx.Logger().Info(
		"frost keysign failure",
		"reason", msg.Blame.FailReason,
		"is_unicast", msg.Blame.IsUnicast,
		"round", msg.Blame.Round,
		"blame", strings.Join(violaters, ", "),
	)

	telemetry.IncrCounterWithLabels(
		[]string{"thornado", "frost", "keysign", "failure"},
		float32(1),
		[]metrics.Label{telemetry.NewLabel("pubkey", msg.PubKey.String()), telemetry.NewLabel("round", msg.Blame.Round)},
	)

	// If at least 2 nodes in the simple majority report round 7 failure freeze the vault.
	// There is a tradeoff here between the number of nodes required to maliciously freeze
	// the vault and the number of nodes required to maliciously prevent freeze - we err
	// on the side of over-freezing.
	if voter.LastRoundCount > 1 || (voter.LastRoundCount > 0 && voter.MemberSignerCount(vaultMemberNodes) <= 2) {
		// this will cause the vault to be "frozen" which causes the
		// rescheduler to NOT reschedule any outbound txns AND cause the tx out
		// manager to not assign new txns to this vault
		for _, coin := range msg.Coins {
			found := false
			for _, chain := range vault.Frozen {
				if chain == coin.Asset.GetChain().String() {
					found = true
					break
				}
			}
			if !found {
				vault.Frozen = append(vault.Frozen, coin.Asset.GetChain().String())
			}
		}
		if err := h.mgr.Keeper().SetVault(ctx, vault); err != nil {
			return nil, fmt.Errorf("fail to save vault: %w", err)
		}
	}

	h.mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, observePenaltyPoints, voter.GetSigners()...)
	voter.Signers = nil
	voter.LastRoundCount = 0
	h.mgr.Keeper().SetFrostKeysignFailVoter(ctx, voter)
	h.markTxOutPendingRetry(ctx, msg)

	penaltyPoints := h.mgr.Keeper().GetConfigInt64(ctx, constants.Keysign_FailPenaltyPoints)
	// fail to generate a new frost key let's penalty the node account

	for _, node := range msg.Blame.BlameNodes {
		nodePubKey, err := common.NewPubKey(node.Pubkey)
		if err != nil {
			return nil, ErrInternal(err, "fail to parse pubkey")
		}
		na, err := h.mgr.Keeper().GetNodeAccountByPubKey(ctx, nodePubKey)
		if err != nil {
			return nil, ErrInternal(err, fmt.Sprintf("fail to get node account,pub key: %s", nodePubKey.String()))
		}
		if err := h.mgr.Keeper().IncNodeAccountPenaltyPoints(penaltyCtx, na.NodeAddress, penaltyPoints); err != nil {
			ctx.Logger().Error("fail to inc penalty points", "error", err)
		}

		if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventPenaltyPoint(na.NodeAddress, penaltyPoints, "fail keysign")); err != nil {
			ctx.Logger().Error("fail to emit penalty point event")
		}
		// go to jail
		ctx.Logger().Info("jailing node", "pubkey", na.PubKeySet.Secp256k1)
		jailTime := getConfigDurationBlocks(ctx, h.mgr.Keeper(), constants.Keysign_FailJailMinutes)
		releaseHeight := ctx.BlockHeight() + jailTime
		reason := "failed to perform keysign"
		if err := h.mgr.Keeper().SetNodeAccountJail(ctx, na.NodeAddress, releaseHeight, reason); err != nil {
			ctx.Logger().Error("fail to set node account jail", "node address", na.NodeAddress, "reason", reason, "error", err)
		}
	}

	return &cosmos.Result{}, nil
}

func (h FrostKeysignHandler) markTxOutPendingRetry(ctx cosmos.Context, msg MsgFrostKeysignFail) {
	txOut, err := h.mgr.Keeper().GetTxOut(ctx, msg.Height)
	if err != nil {
		ctx.Logger().Error("fail to get txout for keysign retry", "height", msg.Height, "error", err)
		return
	}
	if !txOutUsesBatching(*txOut) || txOut.Status != TxOutStatusPendingSign {
		return
	}
	hasPendingForVault := false
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() && item.VaultPubKey.Equals(msg.PubKey) {
			hasPendingForVault = true
			break
		}
	}
	if !hasPendingForVault {
		return
	}
	txOut.Status = TxOutStatusPendingRetry
	txOut.RetryUntilHeight = ctx.BlockHeight() + getConfigDurationBlocks(ctx, h.mgr.Keeper(), constants.Withdrawal_BatchWindowMinutes)
	txOut.SigningLeader = common.EmptyPubKey
	if err := h.mgr.Keeper().SetTxOut(ctx, txOut); err != nil {
		ctx.Logger().Error("fail to set txout pending retry", "height", msg.Height, "error", err)
	}
}

func validateKeysignAuth(ctx cosmos.Context, k keeper.Keeper, signers []cosmos.AccAddress) (cosmos.Context, error) {
	if isSignedByActiveNodeAccounts(ctx, k, signers) {
		return ctx.WithPriority(ActiveNodePriority), nil
	}
	shouldAccept := false
	vaults, err := k.GetBaseVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		return ctx, ErrInternal(err, "fail to get retiring vaults")
	}
	if len(vaults) > 0 {
		for _, signer := range signers {
			nodeAccount, err := k.GetNodeAccount(ctx, signer)
			if err != nil {
				return ctx, ErrInternal(err, "fail to get node account")
			}
			for _, v := range vaults {
				if v.GetMembership().Contains(nodeAccount.PubKeySet.Secp256k1) {
					shouldAccept = true
					break
				}
			}
			if shouldAccept {
				break
			}
		}
	}
	if !shouldAccept {
		return ctx, cosmos.ErrUnauthorized("not authorized")
	}
	return ctx, nil
}

// FrostKeysignAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func FrostKeysignFailAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgFrostKeysignFail) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	expected, err := NewMsgFrostKeysignFail(msg.Height, msg.Blame, msg.Coins, msg.Signer, msg.PubKey)
	if err != nil {
		return ctx, err
	}
	if !strings.EqualFold(expected.ID, msg.ID) {
		return ctx, cosmos.ErrUnknownRequest("invalid keysign fail message")
	}
	newCtx, err := validateKeysignAuth(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	voter, err := k.GetFrostKeysignFailVoter(ctx, msg.ID)
	if err != nil {
		return ctx, err
	}
	if voter.Empty() {
		voter = types.NewFrostKeysignFailVoter(msg.ID, msg.Height)
	}
	if !voter.Sign(msg.Signer) {
		return ctx, cosmos.ErrUnknownRequest("frost keysign failure attestation already submitted")
	}
	if ctx.IsCheckTx() {
		k.SetFrostKeysignFailVoter(ctx, voter)
	}
	return newCtx, nil
}
