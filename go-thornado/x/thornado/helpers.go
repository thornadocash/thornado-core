package thornado

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// isSimulationMode checks if the context indicates we're in simulation mode
func isSimulationMode(ctx cosmos.Context) bool {
	simulationMode, ok := ctx.Value(constants.CtxSimulationMode).(bool)
	return ok && simulationMode
}

func haltBTCVaultForIssue(ctx cosmos.Context, k keeper.Keeper, eventMgr EventManager, tx common.Tx, reason string) error {
	height := ctx.BlockHeight()
	signingKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, common.BTCChain)
	k.SetConfig(ctx, signingKey, height)
	k.SetConfig(ctx, constants.Halt_SolvencyCheck.String(), height)
	if eventMgr == nil {
		return nil
	}
	if err := eventMgr.EmitEvent(ctx, NewEventSecurity(tx, reason)); err != nil {
		ctx.Logger().Error("fail to emit vault halt security event", "error", err)
	}
	if err := eventMgr.EmitEvent(ctx, NewEventSetConfig(signingKey, fmt.Sprintf("%d", height))); err != nil {
		ctx.Logger().Error("fail to emit halt signing config event", "error", err)
	}
	return eventMgr.EmitEvent(ctx, NewEventSetConfig(constants.Halt_SolvencyCheck.String(), fmt.Sprintf("%d", height)))
}

func isStableToStable(ctx cosmos.Context, k keeper.Keeper, source, target common.Asset) bool {
	anchors := k.GetAnchors(ctx, common.BTCAsset)
	if len(anchors) == 0 {
		return false
	}
	sourceL1 := source.GetLayer1Asset()
	targetL1 := target.GetLayer1Asset()
	var sourceIsAnchor, targetIsAnchor bool
	for _, anchor := range anchors {
		if anchor.Equals(sourceL1) {
			sourceIsAnchor = true
		}
		if anchor.Equals(targetL1) {
			targetIsAnchor = true
		}
	}
	return sourceIsAnchor && targetIsAnchor
}

// getMinSlipBps returns artificial slip floor, expressed in basis points (10000).
func getMinSlipBps(
	ctx cosmos.Context,
	k keeper.Keeper,
	asset common.Asset,
	stableOverride bool,
) cosmos.Uint {
	return cosmos.ZeroUint()
}

// isSignedByActiveNodeAccounts check if all signers are active node nodes
func isSignedByActiveNodeAccounts(ctx cosmos.Context, k keeper.Keeper, signers []cosmos.AccAddress) bool {
	if len(signers) == 0 {
		return false
	}
	for _, signer := range signers {
		if signer.Equals(k.GetModuleAccAddress(BaseName)) {
			continue
		}
		if err := signedByActiveNodeAccount(ctx, k, signer); err != nil {
			ctx.Logger().Error("unauthorized account", "error", err)
			return false
		}
	}
	return true
}

func activeNodeAccountsSignerPriority(ctx cosmos.Context, k keeper.Keeper, signers []cosmos.AccAddress) (cosmos.Context, error) {
	if isSignedByActiveNodeAccounts(ctx, k, signers) {
		return ctx.WithPriority(ActiveNodePriority), nil
	}
	return ctx, cosmos.ErrUnauthorized(fmt.Sprintf("%+v are not authorized", signers))
}

// signedByActiveNodeAccounts returns an error unless all signers are active node nodes
func signedByActiveNodeAccount(ctx cosmos.Context, k keeper.Keeper, signer cosmos.AccAddress) error {
	nodeAccount, err := k.GetNodeAccount(ctx, signer)
	if err != nil {
		return fmt.Errorf("error fetching node account: %s: %w", signer.String(), err)
	}
	if nodeAccount.IsEmpty() {
		return fmt.Errorf("node account is unexpectedly empty: %s", signer.String())
	}
	if nodeAccount.Status != NodeActive {
		return fmt.Errorf(
			"node account %s not active: %s",
			signer.String(),
			nodeAccount.Status,
		)
	}
	if nodeAccount.Type != NodeTypeNode {
		return fmt.Errorf(
			"node account %s must be a node: %s",
			signer.String(),
			nodeAccount.Type,
		)
	}

	return nil
}

func wrapError(ctx cosmos.Context, err error, wrap string) error {
	err = fmt.Errorf("%s: %w", wrap, err)
	ctx.Logger().Error(err.Error())
	return multierror.Append(errInternal, err)
}

// addGasFees to gas manager and deduct from vault
func addGasFees(ctx cosmos.Context, mgr Manager, tx ObservedTx) error {
	// If there's no gas, then nothing to do.
	if tx.Tx.Gas.IsEmpty() {
		return nil
	}

	if isTronZeroGasTx(tx) {
		return nil
	}

	// If the transaction wasn't from a known vault, then no relevance for known vaults or pools.
	if !mgr.Keeper().VaultExists(ctx, tx.ObservedPubKey) {
		return nil
	}

	// Since a known vault has spent gas, definitely deduct that gas from the vault's balance
	vault, err := mgr.Keeper().GetVault(ctx, tx.ObservedPubKey)
	if err != nil {
		return err
	}
	vault.SubFunds(tx.Tx.Gas.ToCoins())
	if err := mgr.Keeper().SetVault(ctx, vault); err != nil {
		return err
	}

	// If the vault is inactive, any balance is not represented in active vault
	// accounting, so the Reserve should not reimburse the gas pool.
	if vault.Status == InactiveVault {
		return nil
	}

	// Add the gas to the gas manager to be reimbursed by the Reserve.
	outAsset := common.EmptyAsset
	if len(tx.Tx.Coins) != 0 {
		// Use the first Coin's Asset to indicate the associated outbound Asset for this Gas.
		outAsset = tx.Tx.Coins[0].Asset
	}
	mgr.GasMgr().AddGasAsset(outAsset, tx.Tx.Gas, true)
	return nil
}

func telem(input cosmos.Uint) float32 {
	if !input.BigInt().IsUint64() {
		return 0
	}
	i := input.Uint64()
	return float32(i) / 100000000
}

func telemInt(input cosmos.Int) float32 {
	if !input.BigInt().IsInt64() {
		return 0
	}
	i := input.Int64()
	return float32(i) / 100000000
}

func emitEndBlockTelemetry(ctx cosmos.Context, mgr Manager) error {
	return nil
}

func getEffectiveSecurityBond(nas NodeAccounts) cosmos.Uint {
	amt := cosmos.ZeroUint()
	sort.SliceStable(nas, func(i, j int) bool {
		return nas[i].Bond.LT(nas[j].Bond)
	})
	t := len(nas) * 2 / 3
	if len(nas)%3 == 0 {
		t -= 1
	}
	for i, na := range nas {
		if i <= t {
			amt = amt.Add(na.Bond)
		}
	}
	return amt
}

// Calculates total "effective bond" - the total bond when taking into account the
// Bond-weighted hard-cap
func getTotalEffectiveBond(nas NodeAccounts) (cosmos.Uint, cosmos.Uint) {
	bondHardCap := getHardBondCap(nas)

	totalEffectiveBond := cosmos.ZeroUint()
	for _, item := range nas {
		b := item.Bond
		if item.Bond.GT(bondHardCap) {
			b = bondHardCap
		}

		totalEffectiveBond = totalEffectiveBond.Add(b)
	}

	return totalEffectiveBond, bondHardCap
}

// find the bond size the highest of the bottom 2/3rds node bonds
func getHardBondCap(nas NodeAccounts) cosmos.Uint {
	if len(nas) == 0 {
		return cosmos.ZeroUint()
	}
	sort.SliceStable(nas, func(i, j int) bool {
		return nas[i].Bond.LT(nas[j].Bond)
	})
	i := len(nas) * 2 / 3
	if len(nas)%3 == 0 {
		i -= 1
	}
	return nas[i].Bond
}

// From a list of (active) nodes, get a list of those not in a list (of signers).
func getNonSigners(nas []NodeAccount, signers []cosmos.AccAddress) []cosmos.AccAddress {
	var nonSigners []cosmos.AccAddress
	var signed bool

	for _, na := range nas {
		signed = false
		for _, signer := range signers {
			if na.NodeAddress.Equals(signer) {
				signed = true
				break
			}
		}

		if !signed {
			nonSigners = append(nonSigners, na.NodeAddress)
		}
	}
	return nonSigners
}

// In the case where the max gas of the chain of a queued outbound tx has changed
// Update the ObservedTxVoter so the network can still match the outbound with
// the observed inbound
func updateTxOutGas(ctx cosmos.Context, keeper keeper.Keeper, txOut types.TxOutItem, gas common.Gas) error {
	// When txOut.InHash is 0000000000000000000000000000000000000000000000000000000000000000 , which means the outbound is trigger by the network internally
	// For example , migration, etc. there is no related inbound observation , thus doesn't need to try to find it and update anything
	if txOut.InHash == common.BlankTxID {
		return nil
	}
	voter, err := keeper.GetObservedTxInVoter(ctx, txOut.InHash)
	if err != nil {
		return err
	}

	txOutIndex := -1
	for i, tx := range voter.Actions {
		if tx.Equals(txOut) {
			txOutIndex = i
			voter.Actions[txOutIndex].MaxGas = gas
			keeper.SetObservedTxInVoter(ctx, voter)
			break
		}
	}

	if txOutIndex == -1 {
		return fmt.Errorf("fail to find tx out in ObservedTxVoter %s", txOut.InHash)
	}

	return nil
}

// In the case where the gas rate of the chain of a queued outbound tx has changed
// Update the ObservedTxVoter so the network can still match the outbound with
// the observed inbound
func updateTxOutGasRate(ctx cosmos.Context, keeper keeper.Keeper, txOut types.TxOutItem, gasRate int64) error {
	// When txOut.InHash is 0000000000000000000000000000000000000000000000000000000000000000 , which means the outbound is trigger by the network internally
	// For example , migration, etc. there is no related inbound observation , thus doesn't need to try to find it and update anything
	if txOut.InHash == common.BlankTxID {
		return nil
	}
	voter, err := keeper.GetObservedTxInVoter(ctx, txOut.InHash)
	if err != nil {
		return err
	}

	txOutIndex := -1
	for i, tx := range voter.Actions {
		if tx.Equals(txOut) {
			txOutIndex = i
			voter.Actions[txOutIndex].GasRate = gasRate
			keeper.SetObservedTxInVoter(ctx, voter)
			break
		}
	}

	if txOutIndex == -1 {
		return fmt.Errorf("fail to find tx out in ObservedTxVoter %s", txOut.InHash)
	}

	return nil
}

// atTVLCap - returns bool on if we've hit the TVL hard cap. Coins passed in
// are included in the calculation
func atTVLCap(ctx cosmos.Context, coins common.Coins, mgr Manager) bool {
	return false
}

func isActionsItemDangling(voter ObservedTxVoter, i int) bool {
	if i < 0 || i > len(voter.Actions)-1 {
		// No such Actions item exists in the voter.
		return false
	}

	toi := voter.Actions[i]

	// If any OutTxs item matches an Actions item, deem it to be not dangling.
	for _, outboundTx := range voter.OutTxs {
		// The comparison code is based on matchActionItem, as matchActionItem is unimportable.
		// note: Coins.Contains will match amount as well
		matchCoin := outboundTx.Coins.Contains(toi.Coin)
		if !matchCoin && toi.Coin.Asset.Equals(toi.Chain.GetGasAsset()) {
			asset := toi.Chain.GetGasAsset()
			intendToSpend := toi.Coin.Amount.Add(toi.MaxGas.ToCoins().GetCoin(asset).Amount)
			actualSpend := outboundTx.Coins.GetCoin(asset).Amount.Add(outboundTx.Gas.ToCoins().GetCoin(asset).Amount)
			if intendToSpend.Equal(actualSpend) {
				matchCoin = true
			}
		}
		if toi.ToAddress.Equals(outboundTx.ToAddress) &&
			toi.Chain.Equals(outboundTx.Chain) &&
			matchCoin {
			return false
		}
	}
	return true
}

func IsModuleAccAddress(keeper keeper.Keeper, accAddr cosmos.AccAddress) bool {
	return accAddr.Equals(keeper.GetModuleAccAddress(BaseName)) ||
		accAddr.Equals(keeper.GetModuleAccAddress(BondName)) ||
		accAddr.Equals(keeper.GetModuleAccAddress(ReserveName)) ||
		accAddr.Equals(keeper.GetModuleAccAddress(ModuleName))
}

// getLastChurnHeight returns the block height of the last churn.
func getLastChurnHeight(ctx cosmos.Context, k keeper.Keeper) int64 {
	vaults, err := k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		ctx.Logger().Error("failed to get base vaults", "error", err)
		return 0
	}
	// calculate last churn block height
	var lastChurnHeight int64 // the last block height we had a successful churn
	for _, vault := range vaults {
		if vault.StatusSince > lastChurnHeight {
			lastChurnHeight = vault.StatusSince
		}
	}
	return lastChurnHeight
}

func trimKeyPrefix(key []byte) string {
	keyString := string(key)
	if _, after, found := strings.Cut(keyString, "//"); found {
		return after
	}
	return keyString
}

func IsPeriodLastBlock(ctx cosmos.Context, blocksPerPeriod int64) bool {
	return ctx.BlockHeight()%blocksPerPeriod == 0
}

func isTronZeroGasTx(tx ObservedTx) bool {
	return false
}

// leadingZeros pads a string with leading zeros to reach the specified length.
// If str is already longer than length, it returns the first 'length' characters.
func leadingZeros(length int, str string) string {
	switch {
	case len(str) < length:
		var b strings.Builder
		for i := 1; i <= length-len(str); i++ {
			b.WriteString("0")
		}
		b.WriteString(str)
		return b.String()
	case len(str) > length:
		return str[:length]
	}
	return str
}

func isOutboundFakeGasTx(tx ObservedTx) bool {
	return false
}

// isCancelOrApprovalTx returns true if the observed outbound is a cancel transaction
// sent by bifrost to unstuck a pending transaction or an approval transaction for
// router V6.
//
// Cancel transactions occurs on EVM chains where bifrost needs to replace a stuck
// transaction by sending a new transaction with the same nonce but higher gas price. To
// "cancel" the original transaction, bifrost sends a zero-value transaction to the
// vault's own address. Note: Cancel transactions have amount=0 on the EVM chain, but
// bifrost converts them to DustThreshold when observing to make them observable.
//
// Approval transactions are sent by bifrost to approve token allowances for the router
// contract on behalf of the vault with the ERC20 balance.
func isCancelOrApprovalTx(tx ObservedTx) bool {
	// Must have exactly one coin
	if len(tx.Tx.Coins) != 1 {
		return false
	}

	asset := tx.Tx.Coins[0].Asset

	// Must be an EVM chain
	if !asset.Chain.IsEVM() {
		return false
	}

	// Must be the gas asset
	gasAsset := asset.Chain.GetGasAsset()
	if !asset.Equals(gasAsset) {
		return false
	}

	// Must have amount = DustThreshold (cancel transactions have 0 value on chain,
	// but bifrost scanner converts them to DustThreshold to make them observable)
	dustThreshold := asset.Chain.DustThreshold()
	if !tx.Tx.Coins[0].Amount.Equal(dustThreshold) {
		return false
	}

	return true
}
