package keeperv1

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper/types"
)

func (k KVStore) setNodeAccount(ctx cosmos.Context, key []byte, record NodeAccount) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getNodeAccount(ctx cosmos.Context, key []byte, record *NodeAccount) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, record); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return true, nil
}

// TotalActiveNodes count the number of active node account
func (k KVStore) TotalActiveNodes(ctx cosmos.Context) (int, error) {
	activeNodes, err := k.ListActiveNodes(ctx)
	return len(activeNodes), err
}

// ListNodesWithBond - gets a list of all node node accounts that have bond
// Note: the order of node account in the result is not defined
func (k KVStore) ListNodesWithBond(ctx cosmos.Context) (NodeAccounts, error) {
	nodeAccounts := make(NodeAccounts, 0)
	naIterator := k.GetNodeAccountIterator(ctx)
	defer naIterator.Close()
	for ; naIterator.Valid(); naIterator.Next() {
		var na NodeAccount
		if err := k.cdc.Unmarshal(naIterator.Value(), &na); err != nil {
			return nodeAccounts, dbError(ctx, "Unmarshal: node account", err)
		}
		if na.Type == NodeTypeNode && !na.Bond.IsZero() {
			nodeAccounts = append(nodeAccounts, na)
		}
	}
	return nodeAccounts, nil
}

// ListNodesByStatus - get a list of node node accounts with the given status
func (k KVStore) ListNodesByStatus(ctx cosmos.Context, status NodeStatus) (NodeAccounts, error) {
	nodeAccounts := make(NodeAccounts, 0)
	naIterator := k.GetNodeAccountIterator(ctx)
	defer naIterator.Close()
	for ; naIterator.Valid(); naIterator.Next() {
		var na NodeAccount
		if err := k.cdc.Unmarshal(naIterator.Value(), &na); err != nil {
			return nodeAccounts, dbError(ctx, "Unmarshal: node account", err)
		}
		if na.Type == NodeTypeNode && na.Status == status {
			nodeAccounts = append(nodeAccounts, na)
		}
	}
	return nodeAccounts, nil
}

// ListActiveNodes - get a list of active node node accounts
func (k KVStore) ListActiveNodes(ctx cosmos.Context) (NodeAccounts, error) {
	return k.ListNodesByStatus(ctx, NodeActive)
}

func (k KVStore) RemoveLowBondNodeAccounts(ctx cosmos.Context) error {
	var events cosmos.Events
	lowBondNodes := make([][]byte, 0)
	naIterator := k.GetNodeAccountIterator(ctx)
	defer naIterator.Close()
	for ; naIterator.Valid(); naIterator.Next() {
		var na NodeAccount
		if err := k.cdc.Unmarshal(naIterator.Value(), &na); err != nil {
			return dbError(ctx, "Unmarshal: node account", err)
		}
		if na.Type == NodeTypeVault || na.Status == NodeActive {
			continue
		}
		if na.Type == NodeTypeNode && na.Bond.LTE(cosmos.NewUint(common.One)) {
			if na.Bond.IsZero() {
				lowBondNodes = append(lowBondNodes, naIterator.Key())
				continue
			}
			to, err := na.BondAddress.AccAddress()
			if err != nil {
				return dbError(ctx, "", fmt.Errorf("fail to parse bond address(%s)", na.BondAddress))
			}

			coin := common.NewCoin(common.BTCAsset, na.Bond)
			if err = k.SendFromModuleToAccount(ctx, BondName, to, common.NewCoins(coin)); err != nil {
				ctx.Logger().Error("failed to return bond pool coins", "error", err)
				continue
			}
			bondEvent := NewEventBond(na.Bond, BondReturned, common.Tx{}, &na, to)
			if events, err = bondEvent.Events(); err != nil {
				ctx.Logger().Error("fail to emit bond event", "error", err)
			} else {
				ctx.EventManager().EmitEvents(events)
			}
			lowBondNodes = append(lowBondNodes, naIterator.Key())
		}
	}
	for _, naKey := range lowBondNodes {
		k.del(ctx, naKey)
	}
	return nil
}

// GetMinJoinVersion - get min version to join. Min version is the most popular version
func (k KVStore) GetMinJoinVersion(ctx cosmos.Context) semver.Version {
	type tmpVersionInfo struct {
		version semver.Version
		count   int
	}
	var vCount []tmpVersionInfo
	nodes, err := k.ListActiveNodes(ctx)
	if err != nil {
		_ = dbError(ctx, "Unable to list active node accounts", err)
		return semver.Version{}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].GetVersion().LT(nodes[j].GetVersion())
	})
	for _, na := range nodes {
		exist := false
		for _, item := range vCount {
			if item.version.String() == na.Version {
				exist = true
				break
			}
		}
		if !exist {
			vCount = append(vCount, tmpVersionInfo{
				version: na.GetVersion(),
				count:   0,
			})
		}

		// assume all versions are backward compatible
		for k, v := range vCount {
			if v.version.LTE(na.GetVersion()) {
				v.count++
				vCount[k] = v
			}
		}
	}
	totalCount := len(nodes)
	version := semver.Version{}
	// sort it by version descending
	sort.SliceStable(vCount, func(i, j int) bool {
		return vCount[i].version.GT(vCount[j].version)
	})

	for _, info := range vCount {
		// skip those version that doesn't have majority
		if !HasSuperMajority(info.count, totalCount) {
			continue
		}
		if info.version.GT(version) {
			version = info.version
		}

	}
	return version
}

// GetLowestActiveVersion - get version number of lowest active node
func (k KVStore) GetLowestActiveVersion(ctx cosmos.Context) semver.Version {
	nodes, err := k.ListActiveNodes(ctx)
	if err != nil {
		_ = dbError(ctx, "Unable to list active node accounts", err)
		return constants.SWVersion
	}
	if len(nodes) > 0 {
		version := nodes[0].GetVersion()
		for _, na := range nodes {
			if na.GetVersion().LT(version) {
				version = na.GetVersion()
			}
		}
		return version
	}
	return constants.SWVersion
}

// GetNodeAccount try to get node account with the given address from db
func (k KVStore) GetNodeAccount(ctx cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	emptyPubKeySet := common.PubKeySet{
		Secp256k1: common.EmptyPubKey,
		Ed25519:   common.EmptyPubKey,
	}
	record := NewNodeAccount(addr, NodeUnknown, emptyPubKeySet, "", cosmos.ZeroUint(), "", ctx.BlockHeight())
	_, err := k.getNodeAccount(ctx, k.GetKey(prefixNodeAccount, addr.String()), &record)
	return record, err
}

// GetNodeAccountByPubKey try to get node account with the given pubkey from db
func (k KVStore) GetNodeAccountByPubKey(ctx cosmos.Context, pk common.PubKey) (NodeAccount, error) {
	addr, err := pk.GetThorAddress()
	if err != nil {
		return NodeAccount{}, err
	}
	return k.GetNodeAccount(ctx, addr)
}

// SetNodeAccount save the given node account into data store
func (k KVStore) SetNodeAccount(ctx cosmos.Context, na NodeAccount) error {
	if na.IsEmpty() {
		return nil
	}
	if na.Status == NodeActive {
		if na.ActiveBlockHeight == 0 {
			// the na is active, and does not have a block height when they
			// became active. This must be the first block they are active, so
			// Thornado will set it now.
			na.ActiveBlockHeight = ctx.BlockHeight()
			k.ResetNodeAccountSlashPoints(ctx, na.NodeAddress) // reset slash points
		}
	}

	k.setNodeAccount(ctx, k.GetKey(prefixNodeAccount, na.NodeAddress.String()), na)
	return nil
}

// EnsureNodeKeysUnique check the given consensus pubkey and pubkey set against all the the node account
// return an error when it is overlap with any existing account
func (k KVStore) EnsureNodeKeysUnique(ctx cosmos.Context, signer cosmos.AccAddress, consensusPubKey string, pubKeys common.PubKeySet) error {
	if strings.TrimSpace(consensusPubKey) == "" {
		return dbError(ctx, "", errors.New("Node Consensus Key cannot be empty"))
	}
	if pubKeys.IsEmpty() {
		return dbError(ctx, "", errors.New("PubKeySet cannot be empty"))
	}

	iter := k.GetNodeAccountIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var na NodeAccount
		if err := k.cdc.Unmarshal(iter.Value(), &na); err != nil {
			return dbError(ctx, "Unmarshal: node account", err)
		}
		if na.NodeAddress.Equals(signer) {
			continue
		}
		if na.NodeConsPubKey == consensusPubKey {
			return dbError(ctx, "", fmt.Errorf("%s already exist", na.NodeConsPubKey))
		}
		if na.PubKeySet.Contains(pubKeys.Secp256k1) {
			return dbError(ctx, "", fmt.Errorf("%s already exist", pubKeys))
		}
		if na.PubKeySet.Contains(pubKeys.Ed25519) {
			return dbError(ctx, "", fmt.Errorf("%s already exist", pubKeys))
		}
	}

	return nil
}

// GetNodeAccountIterator iterate node account
func (k KVStore) GetNodeAccountIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixNodeAccount)
}

// GetUpgradeProposalIterator to iterate upgrade proposals.
func (k KVStore) GetUpgradeProposalIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixUpgradeProposals)
}

// GetUpgradeVoteIterator to iterate upgrade votes for a named proposal.
func (k KVStore) GetUpgradeVoteIterator(ctx cosmos.Context, name string) cosmos.Iterator {
	return k.getIterator(ctx, types.DbPrefix(VotePrefix(name)))
}

func VotePrefix(name string) string {
	return fmt.Sprintf("%s%s/", prefixUpgradeVotes, name)
}

// GetNodeAccountSlashPoints - get the slash points associated with the given
// node address
func (k KVStore) GetNodeAccountSlashPoints(ctx cosmos.Context, addr cosmos.AccAddress) (int64, error) {
	record := int64(0)
	_, err := k.getInt64(ctx, k.GetKey(prefixNodeSlashPoints, addr.String()), &record)
	return record, err
}

// SetNodeAccountSlashPoints - set the slash points associated with the given
// node address and uint
func (k KVStore) SetNodeAccountSlashPoints(ctx cosmos.Context, addr cosmos.AccAddress, pts int64) {
	// make sure slash point doesn't go to negative
	if pts < 0 {
		pts = 0
	}
	k.setInt64(ctx, k.GetKey(prefixNodeSlashPoints, addr.String()), pts)
}

// ResetNodeAccountSlashPoints - reset the slash points to zero for associated
// with the given node address
func (k KVStore) ResetNodeAccountSlashPoints(ctx cosmos.Context, addr cosmos.AccAddress) {
	k.del(ctx, k.GetKey(prefixNodeSlashPoints, addr.String()))
}

// IncNodeAccountSlashPoints - increments the slash points associated with the
// given node address and uint
func (k KVStore) IncNodeAccountSlashPoints(ctx cosmos.Context, addr cosmos.AccAddress, pts int64) error {
	current, err := k.GetNodeAccountSlashPoints(ctx, addr)
	if err != nil {
		return err
	}
	k.SetNodeAccountSlashPoints(ctx, addr, current+pts)

	metricLabels, _ := ctx.Context().Value(constants.CtxMetricLabels).([]metrics.Label)
	telemetry.IncrCounterWithLabels(
		[]string{"thornado", "point_slash"},
		float32(pts),
		append(
			metricLabels,
			telemetry.NewLabel("address", addr.String()),
		),
	)

	if config.GetThornado().Telemetry.SlashPoints {
		slashTelemetry(ctx, pts, addr, "IncSlashPoints")
	}

	return nil
}

// DecNodeAccountSlashPoints - decrements the slash points associated with the
// given node address and uint
func (k KVStore) DecNodeAccountSlashPoints(ctx cosmos.Context, addr cosmos.AccAddress, pts int64) error {
	current, err := k.GetNodeAccountSlashPoints(ctx, addr)
	if err != nil {
		return err
	}
	k.SetNodeAccountSlashPoints(ctx, addr, current-pts)

	dec := pts
	if dec > current {
		dec = current
	}

	metricLabels, _ := ctx.Context().Value(constants.CtxMetricLabels).([]metrics.Label)
	telemetry.IncrCounterWithLabels(
		[]string{"thornado", "point_slash_return"},
		float32(dec),
		append(
			metricLabels,
			telemetry.NewLabel("address", addr.String()),
		),
	)

	if config.GetThornado().Telemetry.SlashPoints {
		slashTelemetry(ctx, -pts, addr, "DecSlashPoints")
	}

	return nil
}

func (k KVStore) setJail(ctx cosmos.Context, key []byte, record Jail) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getJail(ctx cosmos.Context, key []byte, record *Jail) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, record); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return true, nil
}

// GetNodeAccountJail - gets jail details for a given node address
func (k KVStore) GetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress) (Jail, error) {
	record := NewJail(addr)
	_, err := k.getJail(ctx, k.GetKey(prefixNodeJail, addr.String()), &record)
	return record, err
}

// SetNodeAccountJail - update the jail details of a node account
func (k KVStore) SetNodeAccountJail(ctx cosmos.Context, addr cosmos.AccAddress, height int64, reason string) error {
	jail, err := k.GetNodeAccountJail(ctx, addr)
	if err != nil {
		return err
	}
	// never reduce sentence
	if jail.ReleaseHeight > height {
		return nil
	}
	jail.ReleaseHeight = height
	jail.Reason = reason

	k.setJail(ctx, k.GetKey(prefixNodeJail, addr.String()), jail)
	return nil
}

// ReleaseNodeAccountFromJail - update the jail details of a node account
func (k KVStore) ReleaseNodeAccountFromJail(ctx cosmos.Context, addr cosmos.AccAddress) error {
	jail, err := k.GetNodeAccountJail(ctx, addr)
	if err != nil {
		return err
	}
	jail.ReleaseHeight = ctx.BlockHeight()
	jail.Reason = ""
	k.setJail(ctx, k.GetKey(prefixNodeJail, addr.String()), jail)
	return nil
}

func (k KVStore) DeductNativeTxFeeFromBond(ctx cosmos.Context, nodeAddr cosmos.AccAddress) error {
	return nil
}
