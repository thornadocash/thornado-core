package thornado

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/blang/semver"
	"github.com/btcsuite/btcd/btcec"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

type TssHandler struct {
	mgr Manager
}

// NewTssHandler create a new handler to process MsgTssPool
func NewTssHandler(mgr Manager) TssHandler {
	return TssHandler{mgr: mgr}
}

// Run it the main entry point to execute Version logic
func (h TssHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgTssPool)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, msg); err != nil {
		ctx.Logger().Error("msg set version failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, msg); err != nil {
		ctx.Logger().Error("fail to process msg set version", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

// verifySecp256K1Signature verifies the provided signature of the public key. This is
// set as a variable so tests can override verification when using random public keys.
var verifySecp256K1Signature = func(pk common.PubKey, sig []byte) error {
	// verify signature length
	if len(sig) != 64 {
		return fmt.Errorf("invalid secp256k1 signature length")
	}

	// build the signature
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	// reject high-S signatures (BIP-62) to prevent signature malleability
	halfOrder := new(big.Int).Rsh(btcec.S256().N, 1)
	if s.Cmp(halfOrder) == 1 {
		return fmt.Errorf("high-S signature rejected (BIP-62)")
	}

	signature := &btcec.Signature{R: r, S: s}

	// verify the signature
	spk, err := pk.Secp256K1()
	if err != nil {
		return fmt.Errorf("fail to get secp256k1 pubkey: %w", err)
	}
	if !signature.Verify([]byte(pk.String()), spk) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func (h TssHandler) validate(ctx cosmos.Context, msg *MsgTssPool) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if msg.KeygenType != BaseVaultKeygen {
		return fmt.Errorf("only base vaults allowed for tss")
	}
	if msg.IsSuccess() {
		minimumMembers := h.mgr.Keeper().GetConfigInt64(ctx, constants.Vault_BaseMembersMin)
		if minimumMembers > 0 && len(msg.PubKeys) < int(minimumMembers) {
			return cosmos.ErrUnknownRequest("not enough members for base vault keygen")
		}
	}

	if !msg.VaultPubKeyEddsa.IsEmpty() {
		if _, err := common.NewPubKey(msg.VaultPubKeyEddsa.String()); err != nil {
			return cosmos.ErrUnknownRequest(err.Error())
		}
	}

	newMsg, err := NewMsgTssPoolV2(msg.PubKeys, msg.VaultPubKey, nil, nil, msg.KeygenType, msg.Height, msg.Blame, msg.Chains, msg.Signer, msg.KeygenTime, msg.VaultPubKeyEddsa, msg.KeysharesBackupEddsa)
	if err != nil {
		return fmt.Errorf("fail to recreate MsgTssPool,err: %w", err)
	}
	if msg.ID != newMsg.ID {
		return cosmos.ErrUnknownRequest("invalid tss message")
	}

	churnRetryBlocks := getConfigDurationBlocks(ctx, h.mgr.Keeper(), constants.Churn_RetryIntervalMinutes)
	if msg.Height <= ctx.BlockHeight()-churnRetryBlocks {
		return cosmos.ErrUnknownRequest("invalid keygen block")
	}

	// verify the check signatures if provided (only a subset of members in signing party)
	if len(msg.Secp256K1Signature) > 0 {
		err = verifySecp256K1Signature(msg.VaultPubKey, msg.Secp256K1Signature)
		if err != nil {
			ctx.Logger().Error(
				"invalid secp256k1 check signature",
				"err", err,
				"ID", msg.ID,
				"signer", msg.Signer.String(),
				"pubkey", msg.VaultPubKey,
				"signature", base64.StdEncoding.EncodeToString(msg.Secp256K1Signature),
			)
			return cosmos.ErrUnknownRequest("invalid secp256k1 check signature")
		}
	}

	keygenBlock, err := h.mgr.Keeper().GetKeygenBlock(ctx, msg.Height)
	if err != nil {
		return fmt.Errorf("fail to get keygen block from data store: %w", err)
	}

	for _, keygen := range keygenBlock.Keygens {
		keyGenMembers := keygen.GetMembers()
		if !msg.GetPubKeys().Equals(keyGenMembers) {
			continue
		}
		// Make sure the keygen type are consistent
		if msg.KeygenType != keygen.Type {
			continue
		}
		for _, member := range keygen.GetMembers() {
			addr, err := member.GetThorAddress()
			if err == nil && addr.Equals(msg.Signer) {
				return validateTssAuth(ctx, h.mgr.Keeper(), msg.Signer)
			}
		}
	}

	return cosmos.ErrUnauthorized("not authorized")
}

func validateTssAuth(ctx cosmos.Context, k keeper.Keeper, signer cosmos.AccAddress) error {
	nodeSigner, err := k.GetNodeAccount(ctx, signer)
	if err != nil {
		return fmt.Errorf("invalid signer")
	}
	if nodeSigner.IsEmpty() {
		return fmt.Errorf("invalid signer")
	}
	if nodeSigner.Status != NodeActive && nodeSigner.Status != NodeSelected {
		return fmt.Errorf("invalid signer status(%s)", nodeSigner.Status)
	}
	return nil
}

func (h TssHandler) handle(ctx cosmos.Context, msg *MsgTssPool) error {
	ctx.Logger().Info("handler tss", "current version", h.mgr.GetVersion())
	blames := make([]string, 0)
	if len(msg.Blame) > 0 {
		var failReason string
		for _, b := range msg.Blame {
			for _, bn := range b.BlameNodes {
				pk, err := common.NewPubKey(bn.Pubkey)
				if err != nil {
					ctx.Logger().Error("fail to get tss keygen pubkey", "pubkey", bn.Pubkey, "error", err)
					continue
				}
				acc, err := pk.GetThorAddress()
				if err != nil {
					ctx.Logger().Error("fail to get tss keygen thor address", "pubkey", bn.Pubkey, "error", err)
					continue
				}
				blames = append(blames, acc.String())
			}
			if len(failReason) == 0 {
				failReason = b.FailReason
			} else {
				failReason = fmt.Sprintf("%s: %s", failReason, b.FailReason)
			}
		}
		sort.Strings(blames)
		ctx.Logger().Info(
			"tss keygen results blame",
			"height", msg.Height,
			"id", msg.ID,
			"pubkey_ecdsa", msg.VaultPubKey,
			"pubkey_eddsa", msg.VaultPubKeyEddsa,
			"blames", strings.Join(blames, ", "),
			"reason", failReason,
			"blamer", msg.Signer,
		)
	}
	// only record TSS metric when keygen is success
	if msg.IsSuccess() && !msg.VaultPubKey.IsEmpty() {
		metric, err := h.mgr.Keeper().GetTssKeygenMetric(ctx, msg.VaultPubKey)
		if err != nil {
			ctx.Logger().Error("fail to get keygen metric", "error", err)
		} else {
			ctx.Logger().Info("save keygen metric to db")
			metric.AddNodeTssTime(msg.Signer, msg.KeygenTime)
			h.mgr.Keeper().SetTssKeygenMetric(ctx, metric)
		}
	}
	voter, err := h.mgr.Keeper().GetTssVoter(ctx, msg.ID)
	if err != nil {
		return fmt.Errorf("fail to get tss voter: %w", err)
	}

	// when VaultPubKey is empty , which means TssVoter with id(msg.ID) doesn't
	// exist before, this is the first time to create it
	// set the VaultPubKey to the one in msg, there is no reason voter.PubKeys
	// have anything in it either, thus override it with msg.PubKeys as well
	if voter.VaultPubKey.IsEmpty() {
		voter.VaultPubKey = msg.VaultPubKey
		voter.VaultPubKeyEddsa = msg.VaultPubKeyEddsa
		voter.PubKeys = msg.PubKeys
	}
	// voter's vault pubkey is the same as the one in message
	if !voter.VaultPubKey.Equals(msg.VaultPubKey) || !voter.VaultPubKeyEddsa.Equals(msg.VaultPubKeyEddsa) {
		return fmt.Errorf("invalid vault pubkey")
	}
	observePenaltyPoints := h.mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := h.mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, h.mgr.Keeper(), constants.Observation_DelayFlexibilityMinutes)

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_tss_pool"),
	}))

	if !voter.Sign(msg.Signer, msg.Chains, string(msg.Secp256K1Signature)) {
		// Penalty for the network having to handle the extra message/s.
		h.mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, msg.Signer)
		ctx.Logger().Info("signer already signed MsgTssPool", "signer", msg.Signer.String(), "txid", msg.ID)
		return nil

	}
	h.mgr.Keeper().SetTssVoter(ctx, voter)

	if !voter.HasConsensus() {
		// Penalty until 2/3rds consensus.
		h.mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, msg.Signer)
		return nil
	}

	if voter.BlockHeight > 0 && (voter.BlockHeight+observeFlex) >= ctx.BlockHeight() {
		// After 2/3rds consensus, only decrement penalty points if within the Observation_DelayFlexibilityMinutes period.
		// (This is expected to only apply for a failed keygen.)
		h.mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, lackOfObservationPenalty, msg.Signer)
	}

	if voter.BlockHeight == 0 {
		// This message brings the voter to 2/3rds consensus.
		// For an IsSuccess() message, BlockHeight and MajorityConsensusBlockHeight will initially be the same.
		voter.BlockHeight = ctx.BlockHeight()
		h.mgr.Keeper().SetTssVoter(ctx, voter)

		// A list of keygen node accounts isn't readily available,
		// so (rather than do a KVStore-check GetNodeAccount)
		// prepare the non-signer AccAddresses manually.
		signers := voter.GetSigners()
		var keygenNodeAccAddresses []cosmos.AccAddress
		for _, member := range msg.PubKeys {
			pkey, err := common.NewPubKey(member)
			if err != nil {
				ctx.Logger().Error("fail to get pub key", "error", err)
				continue
			}
			thorAddr, err := pkey.GetThorAddress()
			if err != nil {
				ctx.Logger().Error("fail to get thor address", "error", err)
				continue
			}
			keygenNodeAccAddresses = append(keygenNodeAccAddresses, thorAddr)
		}
		var nonSigners []cosmos.AccAddress
		var signed bool
		for _, keygenNodeAccAddress := range keygenNodeAccAddresses {
			signed = false
			for _, signer := range signers {
				if keygenNodeAccAddress.Equals(signer) {
					signed = true
					break
				}
			}

			if !signed {
				nonSigners = append(nonSigners, keygenNodeAccAddress)
			}
		}

		// As this signer brings the voter to 2/3rds consensus,
		// increment the signer's penalty points like the before-consensus signers,
		// then decrement all the signers' penalty points and increment the non-signers' penalty points.
		h.mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, msg.Signer)
		h.mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
		h.mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)

		// Do the below only for a non-success message upon 2/3rds consensus.
		if !msg.IsSuccess() {
			// since the keygen failed, it's now safe to reset all nodes in
			// selected status back to standby status
			ready, err := h.mgr.Keeper().ListNodesByStatus(ctx, NodeSelected)
			if err != nil {
				ctx.Logger().Error("fail to get list of selected node accounts", "error", err)
			}
			for _, na := range ready {
				na.UpdateStatus(NodeStandby, ctx.BlockHeight())
				if err := h.mgr.Keeper().SetNodeAccount(ctx, na); err != nil {
					ctx.Logger().Error("fail to set node account", "error", err)
				}
			}

			// If a node fails to join keygen and holds off churn, track it with
			// reliability points. Thornado does not financially penalize bonds.
			penaltyPoints := h.mgr.Keeper().GetConfigInt64(ctx, constants.Keygen_FailPenaltyPoints)
			for _, b := range msg.Blame {
				for _, node := range b.BlameNodes {
					nodePubKey, err := common.NewPubKey(node.Pubkey)
					if err != nil {
						return ErrInternal(err, fmt.Sprintf("fail to parse pubkey(%s)", node.Pubkey))
					}

					na, err := h.mgr.Keeper().GetNodeAccountByPubKey(ctx, nodePubKey)
					if err != nil {
						return fmt.Errorf("fail to get node from it's pub key: %w", err)
					}
					if na.Status == NodeActive {
						failedKeygenPenaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
							telemetry.NewLabel("reason", "failed_keygen"),
						}))
						if err := h.mgr.Keeper().IncNodeAccountPenaltyPoints(failedKeygenPenaltyCtx, na.NodeAddress, penaltyPoints); err != nil {
							ctx.Logger().Error("fail to inc penalty points", "error", err)
						}

						if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventPenaltyPoint(na.NodeAddress, penaltyPoints, "fail keygen")); err != nil {
							ctx.Logger().Error("fail to emit penalty point event")
						}
					} else {
						// go to jail
						jailTime := getConfigDurationBlocks(ctx, h.mgr.Keeper(), constants.Keygen_FailJailMinutes)
						releaseHeight := ctx.BlockHeight() + jailTime
						reason := "failed to perform keygen"
						if err := h.mgr.Keeper().SetNodeAccountJail(ctx, na.NodeAddress, releaseHeight, reason); err != nil {
							ctx.Logger().Error("fail to set node account jail", "node address", na.NodeAddress, "reason", reason, "error", err)
						}

						ctx.Logger().Info("fail keygen; jailed node without bond penalty", "address", na.NodeAddress)
					}
					if err := h.mgr.Keeper().SetNodeAccount(ctx, na); err != nil {
						return fmt.Errorf("fail to save node account: %w", err)
					}
				}

				if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventTssKeygenFailure(b.FailReason, b.Round, b.IsUnicast, msg.Height, blames)); err != nil {
					ctx.Logger().Error("fail to emit keygen failure event")
				}
			}
		}
	}

	// when keygen success
	if msg.IsSuccess() {
		// Separately from the usual consensus-agreement penalty points,
		// those who haven't agreed with a consensus success message incur Keygen_FailPenaltyPoints until agreement.
		judgeLateSigner(ctx, h.mgr, msg, voter)

		// Do the below only for a success message upon complete consensus.
		if voter.HasCompleteConsensus() {
			ctx.Logger().Info(
				"tss keygen results success",
				"height", msg.Height,
				"id", msg.ID,
				"pubkey", msg.VaultPubKey,
			)

			if len(voter.Secp256K1Signatures) > 0 {
				// we must also have quorum on the check signature when it is provided
				consensusSig, ok := voter.ConsensusCheckSignature()
				if !ok {
					ctx.Logger().Error("keygen rejected due to lacking check signature quorum")
					return nil
				}

				// log an error if any bad nodes submitted a mismatched check signature
				for _, sig := range voter.Secp256K1Signatures {
					if sig != consensusSig {
						ctx.Logger().Error(
							"mismatched check signature detected",
							"expected", base64.StdEncoding.EncodeToString([]byte(consensusSig)),
							"found", base64.StdEncoding.EncodeToString([]byte(sig)),
						)
					}
				}
			}

			// Update the BlockHeight to reflect the newly reached state.
			voter.BlockHeight = ctx.BlockHeight()
			h.mgr.Keeper().SetTssVoter(ctx, voter)

			vaultType := BaseVault
			chains := voter.ConsensusChains()
			vault := NewVaultV2(ctx.BlockHeight(), InitVault, vaultType, voter.VaultPubKey, chains.Strings(), voter.VaultPubKeyEddsa)
			vault.Membership = voter.PubKeys

			if err := h.mgr.Keeper().SetVault(ctx, vault); err != nil {
				return fmt.Errorf("fail to save vault: %w", err)
			}
			keygenBlock, err := h.mgr.Keeper().GetKeygenBlock(ctx, msg.Height)
			if err != nil {
				return fmt.Errorf("fail to get keygen block, err: %w, height: %d", err, msg.Height)
			}
			initVaults, err := h.mgr.Keeper().GetBaseVaultsByStatus(ctx, InitVault)
			if err != nil {
				return fmt.Errorf("fail to get init vaults: %w", err)
			}

			var metric *keeper.TssKeygenMetric
			metric, err = h.mgr.Keeper().GetTssKeygenMetric(ctx, msg.VaultPubKey)
			if err != nil {
				ctx.Logger().Error("fail to get keygen metric", "error", err)
			} else {
				var total int64
				for _, item := range metric.NodeTssTimes {
					total += item.TssTime
				}
				evt := NewEventTssKeygenMetric(metric.PubKey, metric.GetMedianTime())
				if err = h.mgr.EventMgr().EmitEvent(ctx, evt); err != nil {
					ctx.Logger().Error("fail to emit tss metric event", "error", err)
				}
			}

			if len(initVaults) == len(keygenBlock.Keygens) {
				ctx.Logger().Info("tss keygen results churn", "baseVaults", len(initVaults))
				for _, v := range initVaults {
					if err = h.mgr.NetworkMgr().RotateVault(ctx, v); err != nil {
						return fmt.Errorf("fail to rotate vault: %w", err)
					}
				}
			} else {
				ctx.Logger().Info("not enough keygen yet", "expecting", len(keygenBlock.Keygens), "current", len(initVaults))
			}

			addrs, err := vault.GetMembership().Addresses()
			members := make([]string, len(addrs))
			if err != nil {
				ctx.Logger().Error("fail to get member addresses", "error", err)
			} else {
				for i, addr := range addrs {
					members[i] = addr.String()
				}
				if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventTssKeygenSuccess(msg.VaultPubKey, msg.Height, members)); err != nil {
					ctx.Logger().Error("fail to emit keygen success event")
				}
			}
		}
	}

	return nil
}

func judgeLateSigner(ctx cosmos.Context, mgr Manager, msg *MsgTssPool, voter TssVoter) {
	// if the voter doesn't reach 2/3 majority consensus , this method should not take any actions
	if !voter.HasConsensus() || !msg.IsSuccess() {
		return
	}
	penaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Keygen_FailPenaltyPoints)
	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_tss_pool"),
	}))

	// when voter already has 2/3 majority signers , restore current message signer's penalty points
	if voter.MajorityConsensusBlockHeight > 0 {
		mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, penaltyPoints, msg.Signer)
		if err := mgr.Keeper().ReleaseNodeAccountFromJail(ctx, msg.Signer); err != nil {
			ctx.Logger().Error("fail to release node account from jail", "node address", msg.Signer, "error", err)
		}
		return
	}

	voter.MajorityConsensusBlockHeight = ctx.BlockHeight()
	mgr.Keeper().SetTssVoter(ctx, voter)
	for _, member := range msg.PubKeys {
		pkey, err := common.NewPubKey(member)
		if err != nil {
			ctx.Logger().Error("fail to get pub key", "error", err)
			continue
		}
		thorAddr, err := pkey.GetThorAddress()
		if err != nil {
			ctx.Logger().Error("fail to get thor address", "error", err)
			continue
		}
		// whoever is in the keygen list , but didn't broadcast MsgTssPool
		if !voter.HasSigned(thorAddr) {
			mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, penaltyPoints, thorAddr)
			// go to jail
			jailTime := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keygen_FailJailMinutes)
			releaseHeight := ctx.BlockHeight() + jailTime
			reason := "failed to vote keygen in time"
			if err := mgr.Keeper().SetNodeAccountJail(ctx, thorAddr, releaseHeight, reason); err != nil {
				ctx.Logger().Error("fail to set node account jail", "node address", thorAddr, "reason", reason, "error", err)
			}
		}
	}
}

// TssAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func TssAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgTssPool) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	if err := validateTssAuth(ctx, k, msg.Signer); err != nil {
		return ctx, err
	}
	voter, err := k.GetTssVoter(ctx, msg.ID)
	if err != nil {
		return ctx, err
	}
	if voter.IsEmpty() {
		voter = NewTssVoter(msg.ID, msg.PubKeys, msg.VaultPubKey, msg.VaultPubKeyEddsa)
	}
	if !voter.Sign(msg.Signer, msg.Chains, string(msg.Secp256K1Signature)) {
		return ctx, cosmos.ErrUnknownRequest("tss attestation already submitted")
	}
	if ctx.IsCheckTx() {
		k.SetTssVoter(ctx, voter)
	}

	return ctx.WithPriority(ActiveNodePriority), nil
}
