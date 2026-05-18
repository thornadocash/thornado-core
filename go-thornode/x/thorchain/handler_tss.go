package thorchain

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

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
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

	if msg.KeygenType != AsgardKeygen {
		return fmt.Errorf("only asgard vaults allowed for tss")
	}

	if !msg.PoolPubKeyEddsa.IsEmpty() {
		if _, err := common.NewPubKey(msg.PoolPubKeyEddsa.String()); err != nil {
			return cosmos.ErrUnknownRequest(err.Error())
		}
	}

	newMsg, err := NewMsgTssPoolV2(msg.PubKeys, msg.PoolPubKey, nil, nil, msg.KeygenType, msg.Height, msg.Blame, msg.Chains, msg.Signer, msg.KeygenTime, msg.PoolPubKeyEddsa, msg.KeysharesBackupEddsa)
	if err != nil {
		return fmt.Errorf("fail to recreate MsgTssPool,err: %w", err)
	}
	if msg.ID != newMsg.ID {
		return cosmos.ErrUnknownRequest("invalid tss message")
	}

	churnRetryBlocks := h.mgr.Keeper().GetConfigInt64(ctx, constants.ChurnRetryInterval)
	if msg.Height <= ctx.BlockHeight()-churnRetryBlocks {
		return cosmos.ErrUnknownRequest("invalid keygen block")
	}

	// verify the check signatures if provided (only a subset of members in signing party)
	if len(msg.Secp256K1Signature) > 0 {
		err = verifySecp256K1Signature(msg.PoolPubKey, msg.Secp256K1Signature)
		if err != nil {
			ctx.Logger().Error(
				"invalid secp256k1 check signature",
				"err", err,
				"ID", msg.ID,
				"signer", msg.Signer.String(),
				"pubkey", msg.PoolPubKey,
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
	if nodeSigner.Status != NodeActive && nodeSigner.Status != NodeReady {
		return fmt.Errorf("invalid signer status(%s)", nodeSigner.Status)
	}
	// ensure we have enough rune
	minBond := k.GetConfigInt64(ctx, constants.MinimumBondInRune)
	if nodeSigner.Bond.LT(cosmos.NewUint(uint64(minBond))) {
		return fmt.Errorf("signer doesn't have enough rune")
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
			"pubkey_ecdsa", msg.PoolPubKey,
			"pubkey_eddsa", msg.PoolPubKeyEddsa,
			"blames", strings.Join(blames, ", "),
			"reason", failReason,
			"blamer", msg.Signer,
		)
	}
	// only record TSS metric when keygen is success
	if msg.IsSuccess() && !msg.PoolPubKey.IsEmpty() {
		metric, err := h.mgr.Keeper().GetTssKeygenMetric(ctx, msg.PoolPubKey)
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

	// when PoolPubKey is empty , which means TssVoter with id(msg.ID) doesn't
	// exist before, this is the first time to create it
	// set the PoolPubKey to the one in msg, there is no reason voter.PubKeys
	// have anything in it either, thus override it with msg.PubKeys as well
	if voter.PoolPubKey.IsEmpty() {
		voter.PoolPubKey = msg.PoolPubKey
		voter.PoolPubKeyEddsa = msg.PoolPubKeyEddsa
		voter.PubKeys = msg.PubKeys
	}
	// voter's pool pubkey is the same as the one in message
	if !voter.PoolPubKey.Equals(msg.PoolPubKey) || !voter.PoolPubKeyEddsa.Equals(msg.PoolPubKeyEddsa) {
		return fmt.Errorf("invalid pool pubkey")
	}
	observeSlashPoints := h.mgr.GetConstants().GetInt64Value(constants.ObserveSlashPoints)
	lackOfObservationPenalty := h.mgr.GetConstants().GetInt64Value(constants.LackOfObservationPenalty)
	observeFlex := h.mgr.Keeper().GetConfigInt64(ctx, constants.ObservationDelayFlexibility)

	slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_tss_pool"),
	}))

	if !voter.Sign(msg.Signer, msg.Chains, string(msg.Secp256K1Signature)) {
		// Slash for the network having to handle the extra message/s.
		h.mgr.Slasher().IncSlashPoints(slashCtx, observeSlashPoints, msg.Signer)
		ctx.Logger().Info("signer already signed MsgTssPool", "signer", msg.Signer.String(), "txid", msg.ID)
		return nil

	}
	h.mgr.Keeper().SetTssVoter(ctx, voter)

	if !voter.HasConsensus() {
		// Slash until 2/3rds consensus.
		h.mgr.Slasher().IncSlashPoints(slashCtx, observeSlashPoints, msg.Signer)
		return nil
	}

	if voter.BlockHeight > 0 && (voter.BlockHeight+observeFlex) >= ctx.BlockHeight() {
		// After 2/3rds consensus, only decrement slash points if within the ObservationDelayFlexibility period.
		// (This is expected to only apply for a failed keygen.)
		h.mgr.Slasher().DecSlashPoints(slashCtx, lackOfObservationPenalty, msg.Signer)
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
		// increment the signer's slash points like the before-consensus signers,
		// then decrement all the signers' slash points and increment the non-signers' slash points.
		h.mgr.Slasher().IncSlashPoints(slashCtx, observeSlashPoints, msg.Signer)
		h.mgr.Slasher().DecSlashPoints(slashCtx, observeSlashPoints, signers...)
		h.mgr.Slasher().IncSlashPoints(slashCtx, lackOfObservationPenalty, nonSigners...)

		// Do the below only for a non-success message upon 2/3rds consensus.
		if !msg.IsSuccess() {
			// since the keygen failed, it's now safe to reset all nodes in
			// ready status back to standby status
			ready, err := h.mgr.Keeper().ListValidatorsByStatus(ctx, NodeReady)
			if err != nil {
				ctx.Logger().Error("fail to get list of ready node accounts", "error", err)
			}
			for _, na := range ready {
				na.UpdateStatus(NodeStandby, ctx.BlockHeight())
				if err := h.mgr.Keeper().SetNodeAccount(ctx, na); err != nil {
					ctx.Logger().Error("fail to set node account", "error", err)
				}
			}

			// if a node fail to join the keygen, thus hold off the network
			// from churning then it will be slashed accordingly
			slashPoints := h.mgr.GetConstants().GetInt64Value(constants.FailKeygenSlashPoints)
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
						failedKeygenSlashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
							telemetry.NewLabel("reason", "failed_keygen"),
						}))
						if err := h.mgr.Keeper().IncNodeAccountSlashPoints(failedKeygenSlashCtx, na.NodeAddress, slashPoints); err != nil {
							ctx.Logger().Error("fail to inc slash points", "error", err)
						}

						if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventSlashPoint(na.NodeAddress, slashPoints, "fail keygen")); err != nil {
							ctx.Logger().Error("fail to emit slash point event")
						}
					} else {
						// go to jail
						jailTime := h.mgr.GetConstants().GetInt64Value(constants.JailTimeKeygen)
						releaseHeight := ctx.BlockHeight() + jailTime
						reason := "failed to perform keygen"
						if err := h.mgr.Keeper().SetNodeAccountJail(ctx, na.NodeAddress, releaseHeight, reason); err != nil {
							ctx.Logger().Error("fail to set node account jail", "node address", na.NodeAddress, "reason", reason, "error", err)
						}

						network, err := h.mgr.Keeper().GetNetwork(ctx)
						if err != nil {
							return fmt.Errorf("fail to get network: %w", err)
						}

						slashBond := network.CalcNodeRewards(cosmos.NewUint(uint64(slashPoints)))
						if slashBond.GT(na.Bond) {
							slashBond = na.Bond
						}
						ctx.Logger().Info("fail keygen , slash bond", "address", na.NodeAddress, "amount", slashBond.String())
						// take out bond from the node account and add it to the Reserve
						// thus good behaviour nodes and liquidity providers will get reward
						na.Bond = common.SafeSub(na.Bond, slashBond)
						coin := common.NewCoin(common.RuneNative, slashBond)
						if !coin.Amount.IsZero() {
							if err := h.mgr.Keeper().SendFromModuleToModule(ctx, BondName, ReserveName, common.NewCoins(coin)); err != nil {
								return fmt.Errorf("fail to transfer funds from bond to reserve: %w", err)
							}
							slashFloat, _ := new(big.Float).SetInt(slashBond.BigInt()).Float32()
							telemetry.IncrCounterWithLabels(
								[]string{"thornode", "bond_slash"},
								slashFloat,
								[]metrics.Label{
									telemetry.NewLabel("address", na.NodeAddress.String()),
									telemetry.NewLabel("reason", "failed_keygen"),
								},
							)
						}

						bondEvent := NewEventBond(slashBond, BondCost, common.Tx{}, &na, nil)
						if err := h.mgr.EventMgr().EmitEvent(ctx, bondEvent); err != nil {
							return fmt.Errorf("fail to emit bond event: %w", err)
						}
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
		// Separately from the usual consensus-agreement slash points,
		// those who haven't agreed with a consensus success message incur FailKeygenSlashPoints until agreement.
		judgeLateSigner(ctx, h.mgr, msg, voter)

		// Do the below only for a success message upon complete consensus.
		if voter.HasCompleteConsensus() {
			ctx.Logger().Info(
				"tss keygen results success",
				"height", msg.Height,
				"id", msg.ID,
				"pubkey", msg.PoolPubKey,
			)

			// we must also have quorum on the check signature
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

			// Update the BlockHeight to reflect the newly reached state.
			voter.BlockHeight = ctx.BlockHeight()
			h.mgr.Keeper().SetTssVoter(ctx, voter)

			vaultType := AsgardVault
			chains := voter.ConsensusChains()
			vault := NewVaultV2(ctx.BlockHeight(), InitVault, vaultType, voter.PoolPubKey, chains.Strings(), h.mgr.Keeper().GetChainContracts(ctx, chains), voter.PoolPubKeyEddsa)
			vault.Membership = voter.PubKeys

			if err := h.mgr.Keeper().SetVault(ctx, vault); err != nil {
				return fmt.Errorf("fail to save vault: %w", err)
			}
			keygenBlock, err := h.mgr.Keeper().GetKeygenBlock(ctx, msg.Height)
			if err != nil {
				return fmt.Errorf("fail to get keygen block, err: %w, height: %d", err, msg.Height)
			}
			initVaults, err := h.mgr.Keeper().GetAsgardVaultsByStatus(ctx, InitVault)
			if err != nil {
				return fmt.Errorf("fail to get init vaults: %w", err)
			}

			var metric *keeper.TssKeygenMetric
			metric, err = h.mgr.Keeper().GetTssKeygenMetric(ctx, msg.PoolPubKey)
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
				ctx.Logger().Info("tss keygen results churn", "asgards", len(initVaults))
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
				if err := h.mgr.EventMgr().EmitEvent(ctx, NewEventTssKeygenSuccess(msg.PoolPubKey, msg.Height, members)); err != nil {
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
	slashPoints := mgr.GetConstants().GetInt64Value(constants.FailKeygenSlashPoints)
	slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_tss_pool"),
	}))

	// when voter already has 2/3 majority signers , restore current message signer's slash points
	if voter.MajorityConsensusBlockHeight > 0 {
		mgr.Slasher().DecSlashPoints(slashCtx, slashPoints, msg.Signer)
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
			mgr.Slasher().IncSlashPoints(slashCtx, slashPoints, thorAddr)
			// go to jail
			jailTime := mgr.GetConstants().GetInt64Value(constants.JailTimeKeygen)
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
	err := validateTssAuth(ctx, k, msg.Signer)
	if err != nil {
		return ctx, err
	}

	return ctx.WithPriority(ActiveNodePriority), nil
}
