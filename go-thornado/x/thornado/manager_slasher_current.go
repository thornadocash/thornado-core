package thornado

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"cosmossdk.io/core/comet"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// Slasher is current implementation of slasher
type SlasherImpl struct {
	keeper   keeper.Keeper
	eventMgr EventManager
}

// newSlasher create a new instance of Slasher
func newSlasher(keeper keeper.Keeper, eventMgr EventManager) *SlasherImpl {
	return &SlasherImpl{keeper: keeper, eventMgr: eventMgr}
}

// BeginBlock called when a new block get proposed to detect whether there are duplicate vote
func (s *SlasherImpl) BeginBlock(ctx cosmos.Context, constAccessor constants.ConstantValues) {
	var doubleSignEvidence []comet.Evidence
	// Iterate through any newly discovered evidence of infraction
	// Slash any nodes (and since-unbonded liquidity within the unbonding period)
	// who contributed to valid infractions
	for i := range ctx.CometInfo().GetEvidence().Len() {
		evidence := ctx.CometInfo().GetEvidence().Get(i)
		switch evidence.Type() {
		case comet.DuplicateVote:
			doubleSignEvidence = append(doubleSignEvidence, evidence)
		default:
			ctx.Logger().Error("ignored unknown evidence type", "type", evidence.Type)
		}
	}

	// Identify nodes which didn't sign the previous block
	var missingSignAddresses []crypto.Address
	var successfulSignAddresses []crypto.Address
	for i := range ctx.CometInfo().GetLastCommit().Votes().Len() {
		voteInfo := ctx.CometInfo().GetLastCommit().Votes().Get(i)
		if voteInfo.GetBlockIDFlag() != comet.BlockIDFlagAbsent {
			successfulSignAddresses = append(successfulSignAddresses, voteInfo.Validator().Address())
		} else {
			missingSignAddresses = append(missingSignAddresses, voteInfo.Validator().Address())
		}
	}

	// Derive Active node node addresses once.
	nas, err := s.keeper.ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to list active nodes", "error", err)
		return
	}
	var nodeAddresses []nodeAddressValidatorAddressPair
	for _, na := range nas {
		pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, na.NodeConsPubKey)
		if err != nil {
			ctx.Logger().Error("fail to derive node address", "error", err)
			continue
		}
		var pair nodeAddressValidatorAddressPair
		pair.nodeAddress = na.NodeAddress
		pair.validatorAddress = pk.Address()
		nodeAddresses = append(nodeAddresses, pair)
	}

	// Act on double signs.
	for _, evidence := range doubleSignEvidence {
		if err := s.HandleDoubleSign(ctx, evidence.Validator().Address(), evidence.Height(), constAccessor, nodeAddresses); err != nil {
			ctx.Logger().Error("fail to slash for double signing a block", "error", err)
		}
	}

	// Act on missing signs.
	for _, missingSignAddress := range missingSignAddresses {
		if err := s.HandleMissingSign(ctx, missingSignAddress, constAccessor, nodeAddresses); err != nil {
			ctx.Logger().Error("fail to slash for missing signing a block", "error", err)
		}
	}

	// Act on successful signs.
	for _, successfulSignAddress := range successfulSignAddresses {
		if err := s.HandleSuccessfulSign(ctx, successfulSignAddress, constAccessor, nodeAddresses); err != nil {
			ctx.Logger().Error("fail to mark for successfully signing a block", "error", err)
		}
	}
}

// HandleDoubleSign - slashes a node for signing two blocks at the same
// block height
// https://blog.cosmos.network/consensus-compare-casper-vs-tendermint-6df154ad56ae
func (s *SlasherImpl) HandleDoubleSign(ctx cosmos.Context, addr crypto.Address, infractionHeight int64, constAccessor constants.ConstantValues, nodeAddresses []nodeAddressValidatorAddressPair) error {
	// check if we're recent enough to slash for this behavior
	maxAge := constAccessor.GetInt64Value(constants.DoubleSignMaxAge)
	if (ctx.BlockHeight() - infractionHeight) > maxAge {
		ctx.Logger().Info("double sign detected but too old to be slashed", "infraction height", fmt.Sprintf("%d", infractionHeight), "address", addr.String())
		return nil
	}

	doubleBlockSignSlashPoints := s.keeper.GetConfigInt64(ctx, constants.DoubleBlockSignSlashPoints)
	for _, pair := range nodeAddresses {
		if addr.String() != pair.validatorAddress.String() {
			continue
		}

		na, err := s.keeper.GetNodeAccount(ctx, pair.nodeAddress)
		if err != nil {
			return err
		}

		slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
			telemetry.NewLabel("address", na.NodeAddress.String()),
			telemetry.NewLabel("reason", "double_block_sign"),
		}))
		if err := s.keeper.IncNodeAccountSlashPoints(slashCtx, na.NodeAddress, doubleBlockSignSlashPoints); err != nil {
			ctx.Logger().Error("fail to increase node account slash point", "error", err, "address", na.NodeAddress.String())
		}

		return s.keeper.SetNodeAccount(ctx, na)
	}

	return fmt.Errorf("could not find active node account with node address: %s", addr)
}

// HandleSuccessfulSign - decrement missing blocks from a node for signing a block
func (s *SlasherImpl) HandleSuccessfulSign(ctx cosmos.Context, addr crypto.Address, constAccessor constants.ConstantValues, nodeAddresses []nodeAddressValidatorAddressPair) error {
	for _, pair := range nodeAddresses {
		if addr.String() != pair.validatorAddress.String() {
			continue
		}

		na, err := s.keeper.GetNodeAccount(ctx, pair.nodeAddress)
		if err != nil {
			return err
		}

		if na.MissingBlocks == 0 {
			return nil
		}

		// decrement the number of blocks that weren't signed
		na.MissingBlocks -= 1

		return s.keeper.SetNodeAccount(ctx, na)
	}

	return fmt.Errorf("could not find active node account with node address: %s", addr)
}

// HandleMissingSign - slashes a node for not signing a block
func (s *SlasherImpl) HandleMissingSign(ctx cosmos.Context, addr crypto.Address, constAccessor constants.ConstantValues, nodeAddresses []nodeAddressValidatorAddressPair) error {
	missBlockSignSlashPoints := s.keeper.GetConfigInt64(ctx, constants.MissBlockSignSlashPoints)
	maxTrack := s.keeper.GetConfigInt64(ctx, constants.MaxTrackMissingBlock)

	for _, pair := range nodeAddresses {
		if addr.String() != pair.validatorAddress.String() {
			continue
		}

		na, err := s.keeper.GetNodeAccount(ctx, pair.nodeAddress)
		if err != nil {
			return err
		}

		slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
			telemetry.NewLabel("address", na.NodeAddress.String()),
			telemetry.NewLabel("reason", "miss_block_sign"),
		}))
		if err := s.keeper.IncNodeAccountSlashPoints(slashCtx, na.NodeAddress, missBlockSignSlashPoints); err != nil {
			ctx.Logger().Error("fail to increase node account slash points", "error", err, "address", na.NodeAddress.String())
		}

		// increment the number of blocks that weren't signed
		na.MissingBlocks += 1
		if na.MissingBlocks > uint64(maxTrack) {
			na.MissingBlocks = uint64(maxTrack)
		}

		return s.keeper.SetNodeAccount(ctx, na)
	}

	return fmt.Errorf("could not find active node account with node address: %s", addr)
}

// LackSigning slash account that fail to sign tx
func (s *SlasherImpl) LackSigning(ctx cosmos.Context, mgr Manager) error {
	var resultErr error
	const maxOutboundAttempts = int64(0)
	signingTransPeriod := mgr.Keeper().GetConfigInt64(ctx, constants.SigningTransactionPeriod)
	if signingTransPeriod == 0 {
		return fmt.Errorf("invalid signing transaction period: %d", signingTransPeriod)
	}

	if ctx.BlockHeight() < signingTransPeriod {
		return nil
	}
	height := ctx.BlockHeight() - signingTransPeriod
	txs, err := s.keeper.GetTxOut(ctx, height)
	if err != nil {
		return fmt.Errorf("fail to get txout from block height(%d): %w", height, err)
	}

	rescheduleHeight := ctx.BlockHeight()

	for i, toi := range txs.TxArray {
		if !common.CurrentChainNetwork.SoftEquals(toi.ToAddress.GetNetwork(toi.Chain)) {
			continue // skip this transaction
		}
		if toi.OutHash.IsEmpty() {
			// Slash node account for not sending funds
			vault, err := s.keeper.GetVault(ctx, toi.VaultPubKey)
			gotVault := err == nil
			if !gotVault {
				// Log and continue with rescheduling; in some edge cases the vault
				// may no longer exist but the item must still be requeued.
				ctx.Logger().Error("Unable to get vault", "error", err, "vault pub key", toi.VaultPubKey.String())
				resultErr = fmt.Errorf("fail to get vault: %w", err)
			}

			// if the vault is frozen, reschedule to same vault with no changes
			frozen := false
			if gotVault && len(vault.Frozen) > 0 {
				var chains common.Chains
				chains, err = common.NewChains(vault.Frozen)
				if err != nil {
					ctx.Logger().Error("failed to convert chains", "error", err)
				}
				if chains.Has(toi.Coin.Asset.GetChain()) {
					etx := common.Tx{
						ID:        toi.InHash,
						Chain:     toi.Chain,
						ToAddress: toi.ToAddress,
						Coins:     []common.Coin{toi.Coin},
						Gas:       toi.MaxGas,
						Memo:      toi.Memo,
					}
					eve := NewEventSecurity(etx, "frozen vault reschedule")
					if err = mgr.EventMgr().EmitEvent(ctx, eve); err != nil {
						ctx.Logger().Error("fail to emit security event", "error", err)
					}
					frozen = true
				}
			}

			var voter ObservedTxVoter
			voter, err = s.keeper.GetObservedTxInVoter(ctx, toi.InHash)
			if err != nil {
				ctx.Logger().Error("fail to get observed tx voter", "error", err)
				resultErr = fmt.Errorf("failed to get observed tx voter: %w", err)
				continue
			}

			// If vault is inactive, check if it still has sufficient funds to fulfill
			// the outbound. If so, reschedule to the same vault (retired vault recovery).
			// If the vault has been drained (e.g. migration completed before this outbound
			// was signed), fall through to active vault selection so the outbound can be
			// fulfilled from a vault that actually holds the funds.
			if gotVault && vault.Status == InactiveVault {
				const maxRetiredVaultRecoveryAttempts = int64(100)
				age := ctx.BlockHeight() - voter.FinalisedHeight
				attempts := age / signingTransPeriod
				if attempts >= maxRetiredVaultRecoveryAttempts {
					ctx.Logger().Info("too many attempts for retired vault recovery",
						"hash", toi.InHash,
						"attempts", attempts,
						"max", maxRetiredVaultRecoveryAttempts)
					continue
				}

				// Check if the inactive vault has enough funds for the outbound.
				// Include gas cost when the gas asset is the same as the outbound asset.
				requiredAmount := toi.Coin.Amount
				for _, gas := range toi.MaxGas {
					if gas.Asset.Equals(toi.Coin.Asset) {
						requiredAmount = requiredAmount.Add(gas.Amount)
					}
				}
				vaultBalance := vault.GetCoin(toi.Coin.Asset).Amount
				if vaultBalance.GTE(requiredAmount) {
					// Vault still has funds — reschedule to the same vault for signing.
					err = mgr.TxOutStore().UnSafeAddTxOutItem(ctx, mgr, toi, rescheduleHeight)
					if err != nil {
						ctx.Logger().Error("fail to add outbound to queue for retired vault recovery", "error", err)
						resultErr = fmt.Errorf("failed to add outbound to queue for retired vault recovery: %w", err)
					} else {
						txs.TxArray[i].OutHash = common.BlankTxID
					}
					continue
				}

				// Vault has been drained — fall through to active vault selection.
				ctx.Logger().Info("inactive vault has insufficient funds, reassigning to active vault",
					"hash", toi.InHash,
					"vault", vault.PubKey,
					"asset", toi.Coin.Asset,
					"required", requiredAmount,
					"available", vaultBalance)
			}

			if maxOutboundAttempts > 0 {
				age := ctx.BlockHeight() - voter.FinalisedHeight
				attempts := age / signingTransPeriod
				if attempts >= maxOutboundAttempts {
					// Skip treasury recovery if there's already an outbound associated or
					// at least one observation of the outbound (outbound is in progress)
					hasOutboundAssociated := len(voter.OutTxs) > 0
					outboundObservations := mgr.Keeper().GetObservedLink(ctx, toi.InHash)
					hasOutboundObservation := len(outboundObservations) > 0
					if hasOutboundAssociated || hasOutboundObservation {
						ctx.Logger().Info("skipping treasury recovery, outbound in progress",
							"hash", toi.InHash,
							"has_outbound", hasOutboundAssociated,
							"has_observation", hasOutboundObservation)
						continue
					}

					treasuryErr := s.sendFailedOutboundToTreasury(ctx, mgr, toi, vault)
					if treasuryErr != nil {
						ctx.Logger().Error("failed to send outbound to treasury recovery", "error", treasuryErr, "hash", toi.InHash)
					} else {
						txs.TxArray[i].OutHash = common.BlankTxID
					}
					continue
				}
			}

			if !frozen && s.needsNewVault(ctx, mgr, vault, signingTransPeriod, voter.FinalisedHeight, toi) {
				var active types.Vaults
				active, err = s.keeper.GetAsgardVaultsByStatus(ctx, ActiveVault)
				if err != nil {
					resultErr = fmt.Errorf("fail to get active asgard vaults: %w", err)
				} else {
					// Deduct the asset's pending outbound funds to represent only the available funds.
					pendingOutbounds := mgr.Keeper().GetPendingOutbounds(ctx, toi.Coin.Asset)
					for i := range active {
						active[i].DeductVaultPendingOutbounds(pendingOutbounds)

						// If the currently-assigned vault is an ActiveVault and the only one with enough funds for the outbound,
						// the item should be reassigned to the same vault rather than assigned to another vault without enough funds;
						// this is especially important for GAIA outbounds, for which insufficient-funds failures
						// have Thornado-unobserved gas costs (causing churn-migration-jamming vault insolvency).
						// As such, re-add the (now free) funds of the outbound being replaced.
						if active[i].PubKey.Equals(toi.VaultPubKey) {
							active[i].Coins = active[i].Coins.Add(toi.Coin)
							active[i].Coins = active[i].Coins.Add(toi.MaxGas...)
						}
					}

					available := active
					mainCoin := toi.Coin
					var maxGasCoin common.Coin
					maxGasCoin, err = mgr.GasMgr().GetMaxGas(ctx, toi.Chain)
					if err != nil {
						ctx.Logger().Error("fail to get max gas", "error", err)
					}
					if mainCoin.Asset.Equals(maxGasCoin.Asset) {
						// If the main coin and the gas coin are the same asset,
						// ensure the assigned vault has enough available for both.
						mainCoin.Amount = mainCoin.Amount.Add(maxGasCoin.Amount)
					} else {
						// If the gas coin isn't the main asset,
						// directly ensure the assigned vault has enough available for it.
						available = active.Has(maxGasCoin)
					}
					available = available.Has(mainCoin)

					if len(available) == 0 {
						// we need to give it somewhere to send from, even if that
						// asgard doesn't have enough funds. This is because if we
						// don't the transaction will just be dropped on the floor,
						// which is bad. Instead it may try to send from an asgard that
						// doesn't have enough funds, fail, and then get rescheduled
						// again later. Maybe by then the network will have enough
						// funds to satisfy.
						// TODO add split logic to send it out from multiple asgards in
						// this edge case.
						ctx.Logger().Error("unable to determine asgard vault to send funds, trying first asgard")
						if len(active) > 0 {
							// Fall back on the vault with the most available funds.
							vault = active.SortBy(mainCoin.Asset)[0]
						}
					} else {
						// Use InHash for vault selection if available, otherwise use block height.
						// For internal protocol transactions (ragnarok, migrations, etc.), InHash
						// is set to BlankTxID since there is no associated inbound transaction.
						inHashValue := toi.InHash.Int64()
						rep := int(inHashValue + ctx.BlockHeight()/signingTransPeriod)
						if vault.PubKey.Equals(available[rep%len(available)].PubKey) {
							// looks like the new vault is going to be the same as the
							// old vault, increment rep to ensure a differ asgard is
							// chosen (if there is more than one option)
							rep++
						}
						vault = available[rep%len(available)]
					}
					voterTx := voter.GetTx(NodeAccounts{})
					if voterTx.IsDone(len(voter.Actions)) {
						if len(voterTx.OutHashes) > 0 && len(voterTx.GetOutHashes()) > 0 {
							txs.TxArray[i].OutHash = voterTx.GetOutHashes()[0]
						}
						continue
					}

					for i, action := range voter.Actions {
						if action.Equals(toi) {
							voter.Actions[i].VaultPubKey = vault.PubKey
							voter.Actions[i].VaultPubKeyEddsa = vault.PubKeyEddsa
							if toi.Aggregator != "" || toi.AggregatorTargetAsset != "" || toi.AggregatorTargetLimit != nil {
								ctx.Logger().Info("clearing aggregator fields on outbound reassignment", "hash", toi.InHash)
								toi.Aggregator = ""
								toi.AggregatorTargetAsset = ""
								toi.AggregatorTargetLimit = nil
								voter.Actions[i].Aggregator = ""
								voter.Actions[i].AggregatorTargetAsset = ""
								voter.Actions[i].AggregatorTargetLimit = nil
							}
						}
					}
					s.keeper.SetObservedTxInVoter(ctx, voter)
					// Save the toi to as a new toi, select Asgard to send it this time.
					toi.VaultPubKey = vault.PubKey
					toi.VaultPubKeyEddsa = vault.PubKeyEddsa

					// update max gas - update voter first, only update txOut if voter succeeds
					if !maxGasCoin.IsEmpty() {
						if err = updateTxOutGas(ctx, s.keeper, toi, common.Gas{maxGasCoin}); err != nil {
							ctx.Logger().Error("Failed to update MaxGas of action in ObservedTxVoter", "hash", toi.InHash, "error", err)
						} else {
							toi.MaxGas = common.Gas{maxGasCoin}
						}
					}
					// Equals checks GasRate so update actions GasRate too (before updating in the queue item)
					// for future updates of MaxGas, which must match for matchActionItem in AddOutTx.
					// Only update txOut GasRate if voter update succeeds to prevent permanent desync.
					gasRate := int64(mgr.GasMgr().GetGasRate(ctx, toi.Chain).Uint64())
					if err = updateTxOutGasRate(ctx, s.keeper, toi, gasRate); err != nil {
						ctx.Logger().Error("Failed to update GasRate of action in ObservedTxVoter", "hash", toi.InHash, "error", err)
					} else {
						toi.GasRate = gasRate
					}
				}
			}

			err = mgr.TxOutStore().UnSafeAddTxOutItem(ctx, mgr, toi, rescheduleHeight)
			if err != nil {
				ctx.Logger().Error("fail to add outbound to queue", "error", err)
				resultErr = fmt.Errorf("failed to add outbound to queue: %w", err)
				continue
			}
			// because the txout item has been rescheduled, thus mark the replaced tx out item as already send out, even it is not
			// in this way bifrost will not send it out again cause node to be slashed
			txs.TxArray[i].OutHash = common.BlankTxID
		}
	}
	if !txs.IsEmpty() {
		if err := s.keeper.SetTxOut(ctx, txs); err != nil {
			return fmt.Errorf("fail to save tx out : %w", err)
		}
	}

	return resultErr
}

// sendFailedOutboundToTreasury records failed outbound recovery for a future
// treasury path.
func (s *SlasherImpl) sendFailedOutboundToTreasury(ctx cosmos.Context, mgr Manager, toi TxOutItem, vault Vault) error {
	ctx.Logger().Info("max attempts reached for outbound; treasury recovery not implemented",
		"hash", toi.InHash,
		"coin", toi.Coin,
		"vault", vault.PubKey)
	return nil
}

// SlashVault thornado keep monitoring the outbound tx from asgard pool
// usually the txout is triggered by thornado itself by
// adding an item into the txout array, refer to TxOutItem for the detail, the
// TxOutItem contains a specific coin and amount.  if somehow thornado
// discover signer send out fund more than the amount specified in TxOutItem,
// it will slash the node account who does that by taking 1.5 * extra fund from
// node account's bond and subsidise the pool that actually lost it.
func (s *SlasherImpl) SlashVault(ctx cosmos.Context, vaultPK common.PubKey, coins common.Coins, mgr Manager) error {
	if coins.IsEmpty() {
		return nil
	}

	vault, err := s.keeper.GetVault(ctx, vaultPK)
	if err != nil {
		return fmt.Errorf("fail to get slash vault (pubkey %s), %w", vaultPK, err)
	}
	membership := vault.GetMembership()

	for _, coin := range coins {
		if coin.IsEmpty() {
			continue
		}
		// Recalculate totalBond for each coin to reflect bond reductions from prior iterations
		totalBond := cosmos.ZeroUint()
		for _, member := range membership {
			na, err := s.keeper.GetNodeAccountByPubKey(ctx, member)
			if err != nil {
				ctx.Logger().Error("fail to get node account bond", "pk", member, "error", err)
				continue
			}
			totalBond = totalBond.Add(na.Bond)
		}

		stolenAssetValue := coin.Amount
		vaultAmount := vault.GetCoin(coin.Asset).Amount
		if stolenAssetValue.GT(vaultAmount) {
			stolenAssetValue = vaultAmount
		}

		stolenRuneValue := stolenAssetValue

		if stolenRuneValue.IsZero() {
			continue
		}

		penaltyPts := mgr.Keeper().GetConfigInt64(ctx, constants.SlashPenalty)
		// total slash amount is penaltyPts the RUNE value of the missing funds
		totalRuneToSlash := common.GetUncappedShare(cosmos.NewUint(uint64(penaltyPts)), cosmos.NewUint(10_000), stolenRuneValue)
		totalRuneSlashed := cosmos.ZeroUint()
		pauseOnSlashThreshold := mgr.Keeper().GetConfigInt64(ctx, constants.PauseOnSlashThreshold)
		if pauseOnSlashThreshold > 0 && totalRuneToSlash.GTE(cosmos.NewUint(uint64(pauseOnSlashThreshold))) {
			// set mimirs to pause signing
			haltsignKey := fmt.Sprintf(constants.MimirTemplateHaltSigning, coin.Asset.Chain)
			s.keeper.SetMimir(ctx, haltsignKey, ctx.BlockHeight())
			mimirEvent1 := NewEventSetMimir(haltsignKey, strconv.FormatInt(ctx.BlockHeight(), 10))
			if err := mgr.EventMgr().EmitEvent(ctx, mimirEvent1); err != nil {
				ctx.Logger().Error("fail to emit set_mimir event", "error", err)
			}
		}
		for _, member := range membership {
			na, err := s.keeper.GetNodeAccountByPubKey(ctx, member)
			if err != nil {
				ctx.Logger().Error("fail to get node account for slash", "pk", member, "error", err)
				continue
			}
			if na.Bond.IsZero() {
				ctx.Logger().Info("node's bond is zero, can't be slashed", "node address", na.NodeAddress.String())
				continue
			}
			runeSlashed := s.slashAndUpdateNodeAccount(ctx, na, coin, vault, totalBond, totalRuneToSlash)
			totalRuneSlashed = totalRuneSlashed.Add(runeSlashed)
		}

		//  2/3 of the total slashed RUNE value to asgard
		//  1/3 of the total slashed RUNE value to reserve
		runeToAsgard := stolenRuneValue

		// stolenRuneValue is the total value in RUNE of stolen coins, but totalRuneSlashed is
		// the total amount able to be slashed from Nodes, credit the pool only totalRuneSlashed
		if totalRuneSlashed.LT(stolenRuneValue) {
			// this should theoretically never happen
			ctx.Logger().Info("total slashed bond amount is less than RUNE value", "slashed_bond", totalRuneSlashed.String(), "rune_value", stolenRuneValue.String())
			runeToAsgard = totalRuneSlashed
		}
		runeToReserve := common.SafeSub(totalRuneSlashed, runeToAsgard)

		if !runeToReserve.IsZero() {
			if err := s.keeper.SendFromModuleToModule(ctx, BondName, ReserveName, common.NewCoins(common.NewCoin(common.RuneAsset(), runeToReserve))); err != nil {
				ctx.Logger().Error("fail to send slash funds to reserve module", "pk", vaultPK, "error", err)
			}
		}
		if !runeToAsgard.IsZero() {
			if err := s.keeper.SendFromModuleToModule(ctx, BondName, AsgardName, common.NewCoins(common.NewCoin(common.RuneAsset(), runeToAsgard))); err != nil {
				ctx.Logger().Error("fail to send slash fund to asgard module", "pk", vaultPK, "error", err)
			}
		}
	}

	return nil
}

// slashAndUpdateNodeAccount slashes a NodeAccount a portion of the value of coin based on their
// portion of the total bond of the offending Vault's membership. Return the amount of RUNE slashed
func (s *SlasherImpl) slashAndUpdateNodeAccount(ctx cosmos.Context, na types.NodeAccount, coin common.Coin, vault types.Vault, totalBond, totalSlashAmountInRune cosmos.Uint) cosmos.Uint {
	slashAmountRune := common.GetSafeShare(na.Bond, totalBond, totalSlashAmountInRune)
	if slashAmountRune.GT(na.Bond) {
		ctx.Logger().Info("slash amount is larger than bond", "slash amount", slashAmountRune, "bond", na.Bond)
		slashAmountRune = na.Bond
	}

	ctx.Logger().Info("slash node account", "node address", na.NodeAddress.String(), "amount", slashAmountRune.String(), "total slash amount", totalSlashAmountInRune)
	na.Bond = common.SafeSub(na.Bond, slashAmountRune)

	bondEvent := NewEventBond(slashAmountRune, BondCost, common.Tx{}, &na, nil)
	if err := s.eventMgr.EmitEvent(ctx, bondEvent); err != nil {
		ctx.Logger().Error("fail to emit bond event", "error", err)
	}

	metricLabels, _ := ctx.Context().Value(constants.CtxMetricLabels).([]metrics.Label)
	slashAmountRuneFloat, _ := new(big.Float).SetInt(slashAmountRune.BigInt()).Float32()
	telemetry.IncrCounterWithLabels(
		[]string{"thornado", "bond_slash"},
		slashAmountRuneFloat,
		append(
			metricLabels,
			telemetry.NewLabel("address", na.NodeAddress.String()),
			telemetry.NewLabel("coin_symbol", coin.Asset.Symbol.String()),
			telemetry.NewLabel("coin_chain", string(coin.Asset.Chain)),
			telemetry.NewLabel("vault_type", vault.Type.String()),
			telemetry.NewLabel("vault_status", vault.Status.String()),
		),
	)

	if err := s.keeper.SetNodeAccount(ctx, na); err != nil {
		ctx.Logger().Error("fail to save node account for slash", "error", err)
	}

	return slashAmountRune
}

// IncSlashPoints will increase the given account's slash points
func (s *SlasherImpl) IncSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		if err := s.keeper.IncNodeAccountSlashPoints(ctx, addr, point); err != nil {
			ctx.Logger().Error("fail to increase node account slash point", "error", err, "address", addr.String())
		}
	}
}

// DecSlashPoints will decrease the given account's slash points
func (s *SlasherImpl) DecSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress) {
	for _, addr := range addresses {
		if err := s.keeper.DecNodeAccountSlashPoints(ctx, addr, point); err != nil {
			ctx.Logger().Error("fail to decrease node account slash point", "error", err, "address", addr.String())
		}
	}
}

// updatePoolFromSlash updates a pool's depths and emits appropriate events after a slash
func (s *SlasherImpl) updatePoolFromSlash(ctx cosmos.Context, asset common.Asset, stolenAsset common.Coin, runeCreditAmt cosmos.Uint, mgr Manager) {
}

func (s *SlasherImpl) needsNewVault(ctx cosmos.Context, mgr Manager, vault Vault, signingTransPeriod, startHeight int64, toi TxOutItem) bool {
	outhashes := mgr.Keeper().GetObservedLink(ctx, toi.InHash)
	if len(outhashes) == 0 {
		return true
	}

	for _, hash := range outhashes {
		voter, err := mgr.Keeper().GetObservedTxOutVoter(ctx, hash)
		if err != nil {
			ctx.Logger().Error("fail to get txout voter", "hash", hash, "error", err)
			continue
		}
		if voter.FinalisedHeight > 0 {
			// Finalised observed txouts should have nothing to do with unfulfilled TxOutItems.
			// This finalised txout might for instance be from an output
			// that was split into multiple outbounds from initially-different vaults.
			continue
		}
		// in the event there are multiple observed txouts for a given inhash, we
		// focus on the matching pubkey and asset
		signers := make(map[string]bool)
		for _, tx1 := range voter.Txs {
			if (!tx1.ObservedPubKey.Equals(toi.VaultPubKey) && !tx1.ObservedPubKey.Equals(toi.VaultPubKeyEddsa)) ||
				len(tx1.Tx.Coins) != 1 ||
				!tx1.Tx.Coins[0].Asset.Equals(toi.Coin.Asset) {
				continue
			}

			for _, tx := range voter.Txs {
				if !tx.Tx.ID.Equals(hash) {
					continue
				}
				for _, signer := range tx.Signers {
					// Uniquely record each signer for this outbound hash.
					signers[signer] = true
				}
			}
		}
		if len(signers) > 0 {
			var count int // count the number of signers from the assigned vault
			for _, member := range vault.Membership {
				pk, err := common.NewPubKey(member)
				if err != nil {
					continue
				}
				addr, err := pk.GetAddress(common.Thornado)
				if err != nil {
					continue
				}
				if _, ok := signers[addr.String()]; ok {
					count++
				}
			}
			// if a simple majority of vault members have observed the outbound,
			// then we should not reschedule. If a vault says it sent it, it
			// sent it and shouldn't get another vault to send it (potentially
			// a second time)
			if count > 0 && HasSimpleMajority(count, len(vault.Membership)) {
				return false
			}
			maxHeight := startHeight + ((int64(len(signers)) + 1) * signingTransPeriod)
			return maxHeight < ctx.BlockHeight()
		}

	}

	return true
}
