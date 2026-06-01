package thornado

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	abci "github.com/cometbft/cometbft/abci/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// NodeMgr is to manage a list of nodes , and rotate them
type NodeMgr struct {
	k             keeper.Keeper
	networkMgr    NetworkManager
	txOutStore    TxOutStore
	eventMgr      EventManager
	existingNodes []string
}

// newNodeMgr create a new instance of NodeMgr
func newNodeMgr(k keeper.Keeper, networkMgr NetworkManager, txOutStore TxOutStore, eventMgr EventManager) *NodeMgr {
	return &NodeMgr{
		k:          k,
		networkMgr: networkMgr,
		txOutStore: txOutStore,
		eventMgr:   eventMgr,
	}
}

// BeginBlock when block begin
func (vm *NodeMgr) BeginBlock(ctx cosmos.Context, mgr Manager, existingNodes []string) error {
	vm.existingNodes = existingNodes
	height := ctx.BlockHeight()
	if height == genesisBlockHeight {
		if err := vm.setupNodeNodes(ctx, height); err != nil {
			return fmt.Errorf("fail to setup node nodes: %w", err)
		}
	}
	lastChurnHeight := getLastChurnHeight(ctx, vm.k)
	churnInterval := getConfigDurationBlocks(ctx, vm.k, constants.Churn_IntervalMinutes)
	churnRetryInterval := getConfigDurationBlocks(ctx, vm.k, constants.Churn_RetryIntervalMinutes)
	// Only compute retry tick if the interval is valid and past the scheduled churn height
	onChurnTick := false
	if churnRetryInterval > 0 && ctx.BlockHeight() > lastChurnHeight+churnInterval {
		onChurnTick = (ctx.BlockHeight()-lastChurnHeight-churnInterval)%churnRetryInterval == 0
	}
	// Allow regular churns to proceed regardless of churnRetryInterval
	isRegularChurn := lastChurnHeight+churnInterval == ctx.BlockHeight()
	if !onChurnTick && !isRegularChurn {
		return nil
	}

	halt := vm.k.GetConfigInt64(ctx, constants.Halt_Churning)
	if halt > 0 && halt <= ctx.BlockHeight() {
		ctx.Logger().Info("churn event skipped due to config has halted churning")
		return nil
	}

	vaults, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		ctx.Logger().Error("Failed to get Base vaults", "error", err)
		return err
	}

	// calculate if we need to retry a churn because we are overdue for a
	// successful one
	nas, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return err
	}
	expectedActiveVaults := int64(0)
	if len(nas) > 0 {
		expectedActiveVaults = 1
	}
	incompleteChurnCheck := int64(len(vaults)) != expectedActiveVaults
	oldVaultCheck := ctx.BlockHeight()-lastChurnHeight > churnInterval
	retryChurn := (oldVaultCheck || incompleteChurnCheck) && onChurnTick

	// skip churn if any active chain is halted
	shouldChurn := lastChurnHeight+churnInterval == ctx.BlockHeight() || retryChurn
	if !shouldChurn {
		return nil
	}

	// collect all chains for active vaults
	activeChains := make(common.Chains, 0)
	for _, v := range vaults {
		activeChains = append(activeChains, v.GetChains()...)
	}
	activeChains = activeChains.Distinct()

	for _, chain := range activeChains {
		if mgr.Keeper().IsChainHalted(ctx, chain) {
			ctx.Logger().Info("Skipping node account rotation for halted chain", "chain", chain)
			return nil
		}
	}

	// don't churn if we have retiring base vaults that still have funds
	retiringVaults, err := vm.k.GetBaseVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		return err
	}
	if len(retiringVaults) > 0 {
		ctx.Logger().Info("Skipping rotation due to retiring vaults still have funds.")
		return nil
	}

	if retryChurn {
		ctx.Logger().Info("Checking for node account rotation... (retry)")
	} else {
		ctx.Logger().Info("Checking for node account rotation...")
	}
	return vm.churn(ctx)
}

func (vm *NodeMgr) churn(ctx cosmos.Context) error {
	cacheCtx, commit := ctx.CacheContext()
	if err := vm.churnInner(cacheCtx); err != nil {
		return err
	}
	commit()
	return nil
}

func (vm *NodeMgr) churnInner(ctx cosmos.Context) error {
	desiredNodeSet := vm.k.GetConfigInt64(ctx, constants.Node_SetDesired)
	redline := vm.k.GetConfigInt64(ctx, constants.Node_BadRedline)
	minSlashPointsForBadNode := vm.k.GetConfigInt64(ctx, constants.Node_PenaltyChurnOutThreshold)

	// update selected actor
	if err := vm.markSelectedActors(ctx); err != nil {
		return err
	}

	// clear leave scores
	if err := vm.clearLeaveScores(ctx); err != nil {
		return err
	}

	// Mark bad, old, low bond, and old version nodes
	// mark someone to get churned out for bad behavior
	_, err := vm.markBadActor(ctx, minSlashPointsForBadNode, redline)
	if err != nil {
		return err
	}

	// mark someone to get churned out for low bond only once the active set is at capacity
	if err = vm.markLowBondActor(ctx, desiredNodeSet); err != nil {
		return err
	}

	// mark someone to get churned out for low version
	if err = vm.markLowVersionNodes(ctx); err != nil {
		return err
	}

	// mark someone to get churned out for age
	if err = vm.markOldActor(ctx); err != nil {
		return err
	}

	// mark someone(s) for not signing blocks
	if err = vm.markMissingActors(ctx); err != nil {
		return err
	}

	next, ok, err := vm.nextVaultNodeAccounts(ctx, int(desiredNodeSet))
	if err != nil {
		return err
	}
	if ok {
		if err := vm.networkMgr.TriggerKeygen(ctx, next); err != nil {
			return err
		}
	}
	return nil
}

// splits given list of node accounts into separate list of nas, for separate
// base vaults
func (vm *NodeMgr) splitNext(ctx cosmos.Context, nas NodeAccounts, baseVaultMembersMinimum int64) []NodeAccounts {
	if baseVaultMembersMinimum <= 0 { // sanity check
		return nil
	}
	// calculate the number of base vaults we'll need to support the given
	// list of node accounts
	groupNum := int64(len(nas)) / baseVaultMembersMinimum
	if int64(len(nas))%baseVaultMembersMinimum > 0 {
		groupNum++
	}
	if groupNum <= 0 { // sanity check
		return nil
	}

	// we want to ensure that a single node operator (designated by bond
	// address) doesn't get too many tss shares for a single Base vault. So we
	// first break out our node accounts into two groups. First, duplicate bond
	// addresses (multi-node operators), and second non-duplicate (single node
	// operators). Then we sort the duplicate group by bond address, then by
	// bond size (large to small). Then we sort the non-duplicate group by bond size (large
	// to small). Then iterate over the first group into base vaults first,
	// then the second group. In the end multi-node operators are spread out
	// against as many base vaults as possible. This also makes it more
	// difficult for a malicious actor to acquire enough spots in a single
	// base to steal as enough are taken by "good actors" that they can't
	// acquire enough tss shares.

	// Check for duplicates
	bondAddrMap := make(map[string]int)
	for _, na := range nas {
		bondAddrMap[na.BondAddress.String()]++
	}
	var duplicateNas, nonDuplicateNas NodeAccounts
	for _, na := range nas {
		if bondAddrMap[na.BondAddress.String()] > 1 {
			duplicateNas = append(duplicateNas, na)
		} else {
			nonDuplicateNas = append(nonDuplicateNas, na)
		}
	}

	sort.SliceStable(duplicateNas, func(i, j int) bool {
		// Check if the bond address counts are the same
		if bondAddrMap[duplicateNas[i].BondAddress.String()] == bondAddrMap[duplicateNas[j].BondAddress.String()] {
			// Check if bond addresses are the same
			if duplicateNas[i].BondAddress.String() == duplicateNas[j].BondAddress.String() {
				// Sort by bond size
				return duplicateNas[i].Bond.GT(duplicateNas[j].Bond)
			}
			// Sort by bond address
			return duplicateNas[i].BondAddress.String() < duplicateNas[j].BondAddress.String()
		}
		// Sort by bond address count
		return bondAddrMap[duplicateNas[i].BondAddress.String()] > bondAddrMap[duplicateNas[j].BondAddress.String()]
	})

	// sort by bond size for non-duplicates
	sort.SliceStable(nonDuplicateNas, func(i, j int) bool {
		return nonDuplicateNas[i].Bond.LT(nonDuplicateNas[j].Bond)
	})

	groups := make([]NodeAccounts, groupNum)
	for i, na := range append(duplicateNas, nonDuplicateNas...) {
		groups[i%len(groups)] = append(groups[i%len(groups)], na)
	}

	// sanity checks
	for i, group := range groups {
		// ensure no group is more than the max
		if int64(len(group)) > baseVaultMembersMinimum {
			ctx.Logger().Info("Skipping rotation due to an Base group is larger than the max size.")
			return nil
		}
		// ensure no group is less than the min
		if int64(len(group)) < 2 {
			ctx.Logger().Info("Skipping rotation due to an Base group is smaller than the min size.")
			return nil
		}
		// ensure a single group is significantly larger than another
		if i > 0 {
			diff := len(groups[i]) - len(groups[i-1])
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				ctx.Logger().Info("Skipping rotation due to an Base groups having dissimilar membership size.")
				return nil
			}
		}
	}

	return groups
}

// EndBlock when block commit
func (vm *NodeMgr) EndBlock(ctx cosmos.Context, mgr Manager) []abci.ValidatorUpdate {
	height := ctx.BlockHeight()
	activeNodes, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to get all active nodes", "error", err)
		return nil
	}

	var newNodes, removedNodes NodeAccounts
	newNodes, removedNodes, err = vm.getChangedNodes(ctx, activeNodes)
	if err != nil {
		ctx.Logger().Error("fail to get node changes", "error", err)
		return nil
	}

	minimumNodesForBFT := vm.k.GetConfigInt64(ctx, constants.Vault_BaseMembersMin)
	nodesAfterChange := len(activeNodes) + len(newNodes) - len(removedNodes)
	if len(activeNodes) >= int(minimumNodesForBFT) && nodesAfterChange < int(minimumNodesForBFT) {
		// Thornado don't have enough nodes for BFT

		// Check we're not migrating funds
		var retiring Vaults
		retiring, err = vm.k.GetBaseVaultsByStatus(ctx, RetiringVault)
		if err != nil {
			ctx.Logger().Error("fail to get retiring vaults", "error", err)
		}

		if len(retiring) == 0 { // wait until all funds are migrated before starting ragnarok
			return nil
		}
	}

	// If there's been a churn (the nodes have changed), continue; if there hasn't, end the function.
	if len(newNodes) == 0 && len(removedNodes) == 0 {
		return nil
	}

	// remove low bond node accounts
	if err = vm.k.RemoveLowBondNodeAccounts(ctx); err != nil {
		ctx.Logger().Error("fail to remove low bond node accounts", "error", err)
	}

	// payout all active node accounts their rewards
	// This including nodes churning out, and takes place before changing the activity status below.
	if err = vm.distributeBondReward(ctx, mgr); err != nil {
		ctx.Logger().Error("fail to pay node bond rewards", "error", err)
	}

	nodes := make([]abci.ValidatorUpdate, 0, len(newNodes)+len(removedNodes))
	for _, na := range newNodes {
		ctx.EventManager().EmitEvent(
			cosmos.NewEvent("UpdateNodeAccountStatus",
				cosmos.NewAttribute("Address", na.NodeAddress.String()),
				cosmos.NewAttribute("Former:", na.Status.String()),
				cosmos.NewAttribute("Current:", NodeActive.String())))
		na.UpdateStatus(NodeActive, height)
		na.LeaveScore = 0
		na.RequestedToLeave = false
		na.MissingBlocks = 0 // zero missing blocks that weren't signed (if any)

		vm.k.ResetNodeAccountSlashPoints(ctx, na.NodeAddress)
		if err = vm.k.SetNodeAccount(ctx, na); err != nil {
			ctx.Logger().Error("fail to save node account", "error", err)
		}
		var pk cryptotypes.PubKey
		pk, err = cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, na.NodeConsPubKey)
		if err != nil {
			ctx.Logger().Error("fail to parse consensus public key", "key", na.NodeConsPubKey, "error", err)
			continue
		}
		nodes = append(nodes, abci.Ed25519ValidatorUpdate(pk.Bytes(), 100))
	}
	for _, na := range removedNodes {
		// retrieve the node from key value store again , as the node might get paid bond, thus the node properties has been changed
		var nodeRemove NodeAccount
		nodeRemove, err = vm.k.GetNodeAccount(ctx, na.NodeAddress)
		if err != nil {
			ctx.Logger().Error("fail to get node account from key value store", "node address", na.NodeAddress)
			continue
		}

		status := NodeStandby
		if nodeRemove.ForcedToLeave {
			status = NodeDisabled
		}
		// if removed node requested to leave , unset it , so they can join back again
		if nodeRemove.RequestedToLeave {
			nodeRemove.RequestedToLeave = false
		}
		ctx.EventManager().EmitEvent(
			cosmos.NewEvent("UpdateNodeAccountStatus",
				cosmos.NewAttribute("Address", nodeRemove.NodeAddress.String()),
				cosmos.NewAttribute("Former:", nodeRemove.Status.String()),
				cosmos.NewAttribute("Current:", status.String())))
		nodeRemove.UpdateStatus(status, height)
		if err = vm.k.SetNodeAccount(ctx, nodeRemove); err != nil {
			ctx.Logger().Error("fail to save node account", "error", err)
		}

		var pk cryptotypes.PubKey
		pk, err = cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, nodeRemove.NodeConsPubKey)
		if err != nil {
			ctx.Logger().Error("fail to parse consensus public key", "key", nodeRemove.NodeConsPubKey, "error", err)
			continue
		}
		caddr := sdk.ValAddress(pk.Address()).String()
		found := false
		for _, exist := range vm.existingNodes {
			if exist == caddr {
				nodes = append(nodes, abci.Ed25519ValidatorUpdate(pk.Bytes(), 0))
				found = true
				break
			}
		}
		if !found {
			ctx.Logger().Info("node is not present, so can't be removed", "node address", caddr)
		}

	}
	// reset all nodes in selected status back to standby status
	ready, err := vm.k.ListNodesByStatus(ctx, NodeSelected)
	if err != nil {
		ctx.Logger().Error("fail to get list of selected node accounts", "error", err)
	}
	for _, na := range ready {
		na.UpdateStatus(NodeStandby, ctx.BlockHeight())
		if err := vm.k.SetNodeAccount(ctx, na); err != nil {
			ctx.Logger().Error("fail to set node account", "error", err)
		}
	}

	// Now that the node statuses have been updated, update the stored MinJoinVersion.
	vm.k.SetMinJoinLast(ctx)

	// On each churn, purge OperationalConfig node votes
	// (without changing the set Config values themselves).
	// This is to stop any OperationalConfig key's threshold for change
	// from creeping up inconveniently high over time.
	// If a small number of nodes repeatedly uses this purge to go against the majority preference,
	// the EconomicConfig Config_OperationalVotesMin could be set to a higher threshold.
	vm.k.PurgeOperationalNodeConfigs(ctx)

	return nodes
}

// getChangedNodes to identify which node had been removed ,and which one had been added
// newNodes , removed nodes,err
func (vm *NodeMgr) getChangedNodes(ctx cosmos.Context, activeNodes NodeAccounts) (NodeAccounts, NodeAccounts, error) {
	var newActive NodeAccounts    // store the list of new active users
	var removedNodes NodeAccounts // nodes that had been removed

	activeVaults, err := vm.k.GetBaseVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		ctx.Logger().Error("fail to get active baseVaults", "error", err)
		return newActive, removedNodes, fmt.Errorf("fail to get active baseVaults: %w", err)
	}
	if len(activeVaults) == 0 {
		return newActive, removedNodes, errors.New("no active vault")
	}
	var membership common.PubKeys
	for _, vault := range activeVaults {
		membership = append(membership, vault.GetMembership()...)
	}

	// find active node accounts that are no longer active
	for _, na := range activeNodes {
		found := false
		for _, vault := range activeVaults {
			if vault.Contains(na.PubKeySet.Secp256k1) {
				found = true
				break
			}
		}
		if na.ForcedToLeave {
			found = false
		}
		if !found && len(membership) > 0 {
			removedNodes = append(removedNodes, na)
		}
	}

	// find selected nodes that change to active
	for _, pk := range membership {
		na, err := vm.k.GetNodeAccountByPubKey(ctx, pk)
		if err != nil {
			ctx.Logger().Error("fail to get node account", "error", err)
			continue
		}
		// Disabled account can't go back , it should not be include in the newActive
		if na.Status != NodeActive && na.Status != NodeDisabled {
			newActive = append(newActive, na)
		}
	}

	return newActive, removedNodes, nil
}

// payNodeAccountBondAward pay
func (vm *NodeMgr) payNodeAccountBondAward(ctx cosmos.Context, lastChurnHeight int64, na NodeAccount, totalBondReward, totalEffectiveBond, bondHardCap cosmos.Uint, mgr Manager) error {
	if na.ActiveBlockHeight == 0 || na.Bond.IsZero() {
		return nil
	}

	network, err := vm.k.GetNetwork(ctx)
	if err != nil {
		return fmt.Errorf("fail to get network: %w", err)
	}

	slashPts, err := vm.k.GetNodeAccountSlashPoints(ctx, na.NodeAddress)
	if err != nil {
		return fmt.Errorf("fail to get node slash points: %w", err)
	}

	// Find number of blocks since the last churn (the last bond reward payout)
	totalActiveBlocks := ctx.BlockHeight() - lastChurnHeight

	// find number of blocks they were well behaved (ie active - slash points)
	earnedBlocks := totalActiveBlocks - slashPts
	if earnedBlocks < 0 {
		earnedBlocks = 0
	}

	naEffectiveBond := na.Bond
	if naEffectiveBond.GT(bondHardCap) {
		naEffectiveBond = bondHardCap
	}

	// reward = totalBondReward * (naEffectiveBond / totalEffectiveBond) * (unslashed blocks since last churn / blocks since last churn)
	reward := common.GetUncappedShare(naEffectiveBond, totalEffectiveBond, totalBondReward)
	reward = common.GetUncappedShare(cosmos.NewUint(uint64(earnedBlocks)), cosmos.NewUint(uint64(totalActiveBlocks)), reward)

	// Add to their bond the amount rewarded
	na.Bond = na.Bond.Add(reward)

	// Minus the number of rune Thornado have awarded them
	network.BondRewardRune = common.SafeSub(network.BondRewardRune, reward)

	// Minus the number of units na has (do not include slash points)
	network.TotalBondUnits = common.SafeSub(
		network.TotalBondUnits,
		cosmos.NewUint(uint64(totalActiveBlocks)),
	)

	if err := vm.k.SetNetwork(ctx, network); err != nil {
		return fmt.Errorf("fail to save network data: %w", err)
	}

	// minus slash points used in this calculation
	vm.k.SetNodeAccountSlashPoints(ctx, na.NodeAddress, slashPts-totalActiveBlocks)

	if err := vm.k.SetNodeAccount(ctx, na); err != nil {
		return fmt.Errorf("fail to save node account: %w", err)
	}
	bondRewardEvent := NewEventBond(reward, BondReward, common.Tx{}, &na, nil)
	if err := mgr.EventMgr().EmitEvent(ctx, bondRewardEvent); err != nil {
		ctx.Logger().Error("fail to emit bond event", "error", err)
	}

	return nil
}

func (vm *NodeMgr) getPendingTxOut(ctx cosmos.Context) (int64, error) {
	signingTransactionPeriod := getConfigDurationBlocks(ctx, vm.k, constants.Keysign_PeriodMinutes)
	startHeight := ctx.BlockHeight() - signingTransactionPeriod
	count := int64(0)
	for height := startHeight; height <= ctx.BlockHeight(); height++ {
		txs, err := vm.k.GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get tx out array from key value store", "error", err)
			return 0, fmt.Errorf("fail to get tx out array from key value store: %w", err)
		}
		for _, tx := range txs.TxArray {
			if tx.OutHash.IsEmpty() {
				count++
			}
		}
	}
	return count, nil
}

func (vm *NodeMgr) distributeBondReward(ctx cosmos.Context, mgr Manager) error {
	var resultErr error
	active, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return fmt.Errorf("fail to get all active node account: %w", err)
	}

	lastChurnHeight := int64(0)
	for _, node := range active {
		if node.ActiveBlockHeight > lastChurnHeight {
			lastChurnHeight = node.ActiveBlockHeight
		}
	}

	totalEffectiveBond, bondHardCap := getTotalEffectiveBond(active)

	network, err := vm.k.GetNetwork(ctx)
	if err != nil {
		return fmt.Errorf("fail to get network: %w", err)
	}

	for _, item := range active {
		if err := vm.payNodeAccountBondAward(ctx, lastChurnHeight, item, network.BondRewardRune, totalEffectiveBond, bondHardCap, mgr); err != nil {
			resultErr = errors.Join(resultErr, err)
			ctx.Logger().Error("fail to pay node account bond award", "node address", item.NodeAddress.String(), "error", err)
		}
	}
	return resultErr
}

func (vm *NodeMgr) ragnarokBond(ctx cosmos.Context, nth int64, mgr Manager) error {
	return nil
}

func (vm *NodeMgr) setupNodeNodes(ctx cosmos.Context, height int64) error {
	if height != genesisBlockHeight {
		ctx.Logger().Info("only need to setup node node when start up", "height", height)
		return nil
	}

	iter := vm.k.GetNodeAccountIterator(ctx)
	defer iter.Close()
	readyNodes := NodeAccounts{}
	activeCandidateNodes := NodeAccounts{}
	for ; iter.Valid(); iter.Next() {
		var na NodeAccount
		if err := vm.k.Cdc().Unmarshal(iter.Value(), &na); err != nil {
			return fmt.Errorf("fail to unmarshal node account, %w", err)
		}
		// when Thornado first start , Thornado only care about these two status
		switch na.Status {
		case NodeSelected:
			readyNodes = append(readyNodes, na)
		case NodeActive:
			activeCandidateNodes = append(activeCandidateNodes, na)
		}
	}
	totalActiveNodes := len(activeCandidateNodes)
	totalNominatedNodes := len(readyNodes)
	if totalActiveNodes == 0 && totalNominatedNodes == 0 {
		return errors.New("no nodes available")
	}

	sort.Sort(activeCandidateNodes)
	sort.Sort(readyNodes)
	activeCandidateNodes = append(activeCandidateNodes, readyNodes...)
	desiredNodeSet := vm.k.GetConfigInt64(ctx, constants.Node_SetDesired)
	for idx, item := range activeCandidateNodes {
		if int64(idx) < desiredNodeSet {
			item.UpdateStatus(NodeActive, ctx.BlockHeight())
		} else {
			item.UpdateStatus(NodeStandby, ctx.BlockHeight())
		}
		if err := vm.k.SetNodeAccount(ctx, item); err != nil {
			return fmt.Errorf("fail to save node account: %w", err)
		}
	}
	return nil
}

func (vm *NodeMgr) getScore(ctx cosmos.Context, slashPts, lastChurnHeight int64) cosmos.Uint {
	// get to the 8th decimal point, but keep numbers integers for safer math
	score := cosmos.NewUint(uint64((ctx.BlockHeight() - lastChurnHeight) * common.One))
	if slashPts == 0 {
		return score
	}
	return score.QuoUint64(uint64(slashPts))
}

// Iterate over active node accounts, finding bad actors with high slash points
func (vm *NodeMgr) findBadActors(ctx cosmos.Context, minSlashPointsForBadNode, badNodeRedline int64) (NodeAccounts, error) {
	badActors := make(NodeAccounts, 0)

	// Guard against division by zero: badNodeRedline is used as a divisor
	// below. If Config sets Node_BadRedline to 0, skip bad actor detection
	// rather than panicking in a consensus-critical code path.
	if badNodeRedline <= 0 {
		return badActors, nil
	}

	nas, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return badActors, err
	}

	if len(nas) == 0 {
		return nil, nil
	}

	// NOTE: Our score gives a numerical representation of the behavior our a
	// node account. The lower the score, the worse behavior. The score is
	// determined by relative to how many slash points they have over how long
	// they have been an active node account.
	type badTracker struct {
		Score       cosmos.Uint
		NodeAccount NodeAccount
	}
	tracker := make([]badTracker, 0, len(nas))
	totalScore := cosmos.ZeroUint()

	// Find bad actor relative to age / slashpoints
	lastChurnHeight := getLastChurnHeight(ctx, vm.k)
	for _, na := range nas {
		slashPts, err := vm.k.GetNodeAccountSlashPoints(ctx, na.NodeAddress)
		if err != nil {
			return badActors, fmt.Errorf("fail to get node slash points: %w", err)
		}

		if slashPts <= minSlashPointsForBadNode {
			continue
		}

		score := vm.getScore(ctx, slashPts, lastChurnHeight)
		totalScore = totalScore.Add(score)

		tracker = append(tracker, badTracker{
			Score:       score,
			NodeAccount: na,
		})
	}

	if len(tracker) == 0 {
		// no offenders, exit nicely
		return nil, nil
	}

	sort.SliceStable(tracker, func(i, j int) bool {
		return tracker[i].Score.LT(tracker[j].Score)
	})

	// score lower is worse
	avgScore := totalScore.QuoUint64(uint64(len(nas)))

	// NOTE: our redline is a hard line in the sand to determine if a node
	// account is sufficiently bad that it should just be removed now. This
	// ensures that if we have multiple "really bad" node accounts, they all
	// can get removed in the same churn. It is important to note we shouldn't
	// be able to churn out more than 1/3rd of our node accounts in a single
	// churn, as that could threaten the security of the funds. This logic to
	// protect against this is not inside this function.
	redline := avgScore.QuoUint64(uint64(badNodeRedline))

	// find any node accounts that have crossed the red line
	for _, track := range tracker {
		if redline.GTE(track.Score) {
			badActors = append(badActors, track.NodeAccount)
		}
	}

	// if no one crossed the redline, lets just grab the worse offender
	if len(badActors) == 0 {
		badActors = NodeAccounts{tracker[0].NodeAccount}
	}

	return badActors, nil
}

// Iterate over active node accounts, finding the one that hasn't been signing blocks
func (vm *NodeMgr) markMissingActors(ctx cosmos.Context) error {
	maxMissingBlocks := vm.k.GetConfigInt64(ctx, constants.Node_MissingBlocksChurnOut)
	maxChurnOut := vm.k.GetConfigInt64(ctx, constants.Node_MissingBlocksChurnOutMax)
	if maxMissingBlocks == 0 || maxChurnOut == 0 {
		return nil // skip this mark
	}

	nas, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return err
	}

	// sort node accounts by number of missing blocks, highest first
	sort.SliceStable(nas, func(i, j int) bool {
		return nas[i].MissingBlocks > nas[j].MissingBlocks
	})

	counter := int64(0)
	for _, n := range nas {
		// Only mark an old actor not already marked for churn-out.
		if n.LeaveScore > 0 {
			continue
		}

		if maxMissingBlocks < int64(n.MissingBlocks) {
			if err := vm.markActor(ctx, n, "for not signing blocks"); err != nil {
				return err
			}
			counter += 1
			if counter >= maxChurnOut {
				break
			}
		}
	}

	return nil
}

// Iterate over active node accounts, finding the one that has been active longest
func (vm *NodeMgr) findOldActor(ctx cosmos.Context) (NodeAccount, error) {
	na := NodeAccount{}
	nas, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return na, err
	}

	na.StatusSince = ctx.BlockHeight() // set the start status age to "now"
	for _, n := range nas {
		// Only mark an old actor not already marked for churn-out.
		if n.LeaveScore > 0 {
			continue
		}
		if n.StatusSince < na.StatusSince {
			na = n
		}
	}

	return na, nil
}

// Iterate over active node accounts, finding the one that has the lowest bond
func (vm *NodeMgr) findLowBondActor(ctx cosmos.Context) (NodeAccount, error) {
	na := NodeAccount{}
	nas, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return na, err
	}

	first := true
	for _, n := range nas {
		// Only mark a low bond actor not already marked for churn-out.
		if n.LeaveScore > 0 {
			continue
		}
		if first || n.Bond.LT(na.Bond) {
			na = n
			first = false
		}
	}

	return na, nil
}

// Mark an actor to be churned out
func (vm *NodeMgr) markActor(ctx cosmos.Context, na NodeAccount, reason string) error {
	if !na.IsEmpty() && na.LeaveScore == 0 {
		ctx.Logger().Info("marked Node to be churned out", "node address", na.NodeAddress, "reason", reason)
		slashPts, err := vm.k.GetNodeAccountSlashPoints(ctx, na.NodeAddress)
		if err != nil {
			return fmt.Errorf("fail to get node account(%s) slash points: %w", na.NodeAddress, err)
		}
		na.LeaveScore = vm.getScore(ctx, slashPts, getLastChurnHeight(ctx, vm.k)).Uint64()
		return vm.k.SetNodeAccount(ctx, na)
	}
	return nil
}

// Mark an old actor to be churned out
func (vm *NodeMgr) markOldActor(ctx cosmos.Context) error {
	na, err := vm.findOldActor(ctx)
	if err != nil {
		return err
	}
	if err := vm.markActor(ctx, na, "for age"); err != nil {
		return err
	}
	return nil
}

// Mark a low bond actor to be churned out once the active set is at capacity.
func (vm *NodeMgr) markLowBondActor(ctx cosmos.Context, desiredNodeSet int64) error {
	if desiredNodeSet > 0 {
		nas, err := vm.k.ListActiveNodes(ctx)
		if err != nil {
			return err
		}
		if int64(len(nas)) < desiredNodeSet {
			return nil
		}
	}

	na, err := vm.findLowBondActor(ctx)
	if err != nil {
		return err
	}
	if err := vm.markActor(ctx, na, "for low bond"); err != nil {
		return err
	}
	return nil
}

// Mark a bad actor to be churned out
func (vm *NodeMgr) markBadActor(ctx cosmos.Context, minSlashPointsForBadNode, redline int64) (int64, error) {
	nas, err := vm.findBadActors(ctx, minSlashPointsForBadNode, redline)
	if err != nil {
		return 0, err
	}
	for _, na := range nas {
		if err := vm.markActor(ctx, na, "for bad behavior"); err != nil {
			return 0, err
		}
	}
	return int64(len(nas)), nil
}

// Mark low-version nodes as candidates to churn out.
func (vm *NodeMgr) markLowVersionNodes(ctx cosmos.Context) error {
	_, minJoinLastHeight := vm.k.GetMinJoinLast(ctx)
	if ctx.BlockHeight() < minJoinLastHeight+21600 {
		return nil
	}

	nodeAccs, err := vm.findLowVersionNodes(ctx, 1)
	if err != nil {
		return err
	}
	if len(nodeAccs) > 0 {
		for _, na := range nodeAccs {
			if err := vm.markActor(ctx, na, "for version lower than minimum join version"); err != nil {
				return err
			}
		}
	}
	return nil
}

// Finds up to `maxNodesToFind` active nodes with version lower than the most "popular" version
func (vm *NodeMgr) findLowVersionNodes(ctx cosmos.Context, maxNodesToFind int64) (NodeAccounts, error) {
	minimumVersion, _ := vm.k.GetMinJoinLast(ctx)
	activeNodes, err := vm.k.ListNodesByStatus(ctx, NodeActive)
	if err != nil {
		return NodeAccounts{}, err
	}
	nodeAccs := NodeAccounts{}
	for _, na := range activeNodes {
		// Only mark low version actors not already marked for churn-out.
		if na.LeaveScore > 0 {
			continue
		}
		if na.GetVersion().LT(minimumVersion) {
			nodeAccs = append(nodeAccs, na)
		}
		if len(nodeAccs) == int(maxNodesToFind) {
			return nodeAccs, nil
		}
	}
	return nodeAccs, nil
}

// clearLeaveScores - clears all leaves scores of active nodes except for
// ones that requested to leave
func (vm *NodeMgr) clearLeaveScores(ctx cosmos.Context) error {
	active, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return err
	}

	for _, na := range active {
		if na.RequestedToLeave || na.ForcedToLeave {
			continue
		}
		na.LeaveScore = 0

		if err := vm.k.SetNodeAccount(ctx, na); err != nil {
			return err
		}
	}

	return nil
}

// find the actor selected for the next node slot
func (vm *NodeMgr) markSelectedActors(ctx cosmos.Context) error {
	candidates := NodeAccounts{}
	for _, status := range []NodeStatus{NodeWhiteListed, NodeStandby, NodeSelected} {
		nodes, err := vm.k.ListNodesByStatus(ctx, status)
		if err != nil {
			return err
		}
		candidates = append(candidates, nodes...)
	}

	selected := vm.selectHighestBondedNode(ctx, candidates)
	for _, na := range candidates {
		status, _ := vm.NodeAccountPreflightCheck(ctx, na, vm.k.GetConstants())
		if status == NodeSelected && !na.NodeAddress.Equals(selected.NodeAddress) {
			status = NodeWhiteListed
		}
		na.UpdateStatus(status, ctx.BlockHeight())

		if err := vm.k.SetNodeAccount(ctx, na); err != nil {
			return err
		}
	}

	return nil
}

func (vm *NodeMgr) selectHighestBondedNode(ctx cosmos.Context, candidates NodeAccounts) NodeAccount {
	var selected NodeAccount
	for _, na := range candidates {
		status, err := vm.NodeAccountPreflightCheck(ctx, na, vm.k.GetConstants())
		if err != nil || status != NodeSelected {
			continue
		}
		if selected.IsEmpty() || na.Bond.GT(selected.Bond) {
			selected = na
			continue
		}
		if na.Bond.Equal(selected.Bond) && na.NodeAddress.String() < selected.NodeAddress.String() {
			selected = na
		}
	}
	return selected
}

// NodeAccountPreflightCheck preflight check to find out what the node account's next status will be
func (vm *NodeMgr) NodeAccountPreflightCheck(ctx cosmos.Context, na NodeAccount, _ constants.ConfigValues) (NodeStatus, error) {
	// ensure banned nodes can't get churned in again
	if na.ForcedToLeave {
		return NodeDisabled, fmt.Errorf("node account has been banned")
	}

	// Check if they've requested to leave
	if na.RequestedToLeave {
		return NodeStandby, fmt.Errorf("node account has requested to leave")
	}

	if na.Maintenance {
		return NodeStandby, fmt.Errorf("node account is in maintenance mode")
	}

	// Check that the node account has an IP address
	if net.ParseIP(na.IPAddress) == nil {
		return NodeStandby, fmt.Errorf("node account has invalid registered IP address")
	}

	// Check that the node account has an pubkey set
	if na.PubKeySet.IsEmpty() {
		return NodeWhiteListed, fmt.Errorf("node account has not registered their pubkey set")
	}

	requiredBond := vm.requiredBondForNode(ctx, na)
	if na.Bond.LT(requiredBond) {
		return NodeWhiteListed, fmt.Errorf("insufficient bond: %d/%d", na.Bond.Uint64(), requiredBond.Uint64())
	}

	minVersion, _ := vm.k.GetMinJoinLast(ctx)
	// Check version number is still supported
	if na.GetVersion().LT(minVersion) {
		return NodeStandby, fmt.Errorf("node account does not meet min version requirement: %s vs %s", na.Version, minVersion)
	}

	jail, err := vm.k.GetNodeAccountJail(ctx, na.NodeAddress)
	if err != nil {
		ctx.Logger().Error("fail to get node account jail", "error", err)
		return NodeStandby, fmt.Errorf("cannot fetch jail status: %w", err)
	}
	if jail.IsJailed(ctx) {
		return NodeStandby, fmt.Errorf("node account is jailed until block %d: %s", jail.ReleaseHeight, jail.Reason)
	}

	return NodeSelected, nil
}

func (vm *NodeMgr) requiredBondForNode(ctx cosmos.Context, na NodeAccount) cosmos.Uint {
	slot := uint64(0)
	if strings.TrimSpace(na.NodeConsPubKey) != "" {
		if bond, err := vm.k.GetShielderNodeBond(ctx, na.NodeConsPubKey); err == nil && bond.NodePubKey != "" {
			slot = bond.Slot
		}
	}
	return cosmos.NewUint(shielderBondRequiredSats(ctx, vm.k, slot))
}

// Returns a list of nodes to include in the next pool
func (vm *NodeMgr) nextVaultNodeAccounts(ctx cosmos.Context, targetCount int) (NodeAccounts, bool, error) {
	rotation := false // track if are making any changes to the current active node accounts

	ready, err := vm.k.ListNodesByStatus(ctx, NodeSelected)
	if err != nil {
		return nil, false, err
	}

	// sort by bond size, descending
	sort.SliceStable(ready, func(i, j int) bool {
		return ready[i].Bond.GT(ready[j].Bond)
	})

	active, err := vm.k.ListActiveNodes(ctx)
	if err != nil {
		return nil, false, err
	}

	// find out all the nodes that had been marked to leave , and update their score again , because even after a node has been marked
	// to be churn out , they can continue to accumulate slash points, in the scenario that an active node go offline , and consistently fail
	// keygen / keysign for a while , we would like to churn it out first
	lastChurnHeight := getLastChurnHeight(ctx, vm.k)
	for idx, item := range active {

		if item.LeaveScore == 0 {
			continue
		}
		var slashPts int64
		slashPts, err = vm.k.GetNodeAccountSlashPoints(ctx, item.NodeAddress)
		if err != nil {
			ctx.Logger().Error("fail to get node account slash points", "error", err, "node address", item.NodeAddress.String())
			continue
		}
		newScore := vm.getScore(ctx, slashPts, lastChurnHeight)
		if !newScore.IsZero() {
			active[idx].LeaveScore = newScore.Uint64()
		}
	}

	// sort by LeaveScore ascending
	// giving preferential treatment to people who are forced to leave
	//  and then requested to leave
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].ForcedToLeave != active[j].ForcedToLeave {
			return active[i].ForcedToLeave
		}
		if active[i].RequestedToLeave != active[j].RequestedToLeave {
			return active[i].RequestedToLeave
		}
		// sort by LeaveHeight ascending , but exclude LeaveHeight == 0 , because that's the default value
		if active[i].LeaveScore == 0 && active[j].LeaveScore > 0 {
			return false
		}
		if active[i].LeaveScore > 0 && active[j].LeaveScore == 0 {
			return true
		}
		return active[i].LeaveScore < active[j].LeaveScore
	})

	toRemove := findCountToRemove(active)
	if toRemove > 0 {
		rotation = true
		active = active[toRemove:]
	}
	// add selected nodes to become active
	limit := toRemove + 1
	minimumNodesForBFT := vm.k.GetConfigInt64(ctx, constants.Vault_BaseMembersMin)
	if len(active)+limit < int(minimumNodesForBFT) {
		limit = int(minimumNodesForBFT) - len(active)
	}

	for i := 1; targetCount > len(active); i++ {
		if len(ready) >= i {
			rotation = true
			active = append(active, ready[i-1])
		}
		if i >= limit || i > len(ready) { // limit adding selected accounts
			break
		}
	}

	return active, rotation, nil
}
