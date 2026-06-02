package keeperv1

import (
	"bytes"
	"fmt"
	"strings"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/runtime"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// GetUpgradePlan proxies through to the upgrade keeper
func (k KVStore) GetUpgradePlan(ctx cosmos.Context) (upgradetypes.Plan, error) {
	return k.upgradeKeeper.GetUpgradePlan(ctx)
}

// ScheduleUpgrade proxies through to the upgrade keeper
func (k KVStore) ScheduleUpgrade(ctx cosmos.Context, plan upgradetypes.Plan) error {
	return k.upgradeKeeper.ScheduleUpgrade(ctx, plan)
}

// ClearUpgradePlan proxies through to the upgrade keeper
func (k KVStore) ClearUpgradePlan(ctx cosmos.Context) error {
	return k.upgradeKeeper.ClearUpgradePlan(ctx)
}

// ProposeUpgrade proposes an upgrade by name
func (k KVStore) ProposeUpgrade(ctx cosmos.Context, name string, upgrade types.UpgradeProposal) error {
	key := fmt.Sprintf("%s%s", prefixUpgradeProposals, name)
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	v, err := k.cdc.Marshal(&upgrade)
	if err != nil {
		return fmt.Errorf("failed to marshal proposed upgrade: %w", err)
	}

	store.Set([]byte(key), v)

	return nil
}

// GetProposedUpgrade retrieves a proposed upgrade
func (k KVStore) GetProposedUpgrade(ctx cosmos.Context, name string) (*types.UpgradeProposal, error) {
	key := []byte(fmt.Sprintf("%s%s", prefixUpgradeProposals, name))
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	v := store.Get(key)
	if v == nil {
		return nil, nil
	}

	var upgrade types.UpgradeProposal
	if err := k.cdc.Unmarshal(v, &upgrade); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proposed upgrade: %w", err)
	}

	return &upgrade, nil
}

// GetUpgradeVote retrieves a vote from a node for an upgrade proposal.
func (k KVStore) GetUpgradeVote(ctx cosmos.Context, addr cosmos.AccAddress, name string) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	v := store.Get(append([]byte(VotePrefix(name)), addr...))
	if v == nil {
		return false, fmt.Errorf("no vote found on proposal %s for %s", name, addr)
	}

	return bytes.Equal(v, []byte{0x1}), nil
}

// ApproveUpgrade approves an upgrade as a node
func (k KVStore) ApproveUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	store.Set(append([]byte(VotePrefix(name)), addr...), []byte{0x1})
}

// RejectUpgrade rejects an upgrade as a node
func (k KVStore) RejectUpgrade(ctx cosmos.Context, addr cosmos.AccAddress, name string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	store.Set(append([]byte(VotePrefix(name)), addr...), []byte{0xFF})
}

// RemoveExpiredUpgradeProposals removes an upgrade proposal and all votes
// after the proposal height has passed.
func (k KVStore) RemoveExpiredUpgradeProposals(ctx cosmos.Context) error {
	iter := k.GetUpgradeProposalIterator(ctx)
	defer iter.Close()

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	for ; iter.Valid(); iter.Next() {
		key, value := iter.Key(), iter.Value()

		nameSplit := strings.Split(string(key), "/")
		name := nameSplit[len(nameSplit)-1]

		var upgrade types.Upgrade
		if err := k.cdc.Unmarshal(value, &upgrade); err != nil {
			return fmt.Errorf("failed to unmarshal proposed upgrade: %w", err)
		}

		if ctx.BlockHeight() <= upgrade.Height {
			continue
		}

		ctx.Logger().Info(
			"Deleting expired upgrade proposal",
			"name", name,
		)

		k.removeExpiredUpgradeProposalVotes(ctx, name)
		store.Delete(key)
	}

	return nil
}

func (k KVStore) removeExpiredUpgradeProposalVotes(ctx cosmos.Context, name string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))

	iter := k.GetUpgradeVoteIterator(ctx, name)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		store.Delete(iter.Key())
	}
}

// UpgradeQuorum represents the quorum status of an upgrade proposal.
type UpgradeQuorum struct {
	Approved        bool
	ApprovingVals   int
	TotalActive     int
	NeededForQuorum int
}

// UpgradeApprovedByMajority returns true and no error if the upgrade is approved by 2/3 of Nodes.
// it additionally returns the current approving val count, the total active val count, and the
// additional active nodes needed to reach quorum, if not already approved.
func UpgradeApprovedByMajority(ctx cosmos.Context, k keeper.Keeper, name string) (*UpgradeQuorum, error) {
	activeVals, err := k.ListActiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list active nodes: %w", err)
	}

	active := make(map[string]bool)

	for _, na := range activeVals {
		active[na.NodeAddress.String()] = true
	}

	totalActive := len(active)

	iterV := k.GetUpgradeVoteIterator(ctx, name)
	defer iterV.Close()

	var approvingVals int

	for ; iterV.Valid(); iterV.Next() {
		key, vote := iterV.Key(), iterV.Value()
		if !bytes.Equal(vote, []byte{0x1}) {
			continue
		}

		prefix := []byte(VotePrefix(name))
		addr := cosmos.AccAddress(bytes.TrimPrefix(key, prefix))

		_, ok := active[addr.String()]
		if !ok {
			// this could happen if a node votes and then becomes inactive
			continue
		}

		approvingVals++
	}

	// Use integer arithmetic for deterministic consensus: approvingVals >= totalActive*2/3
	// is equivalent to approvingVals*3 >= totalActive*2 (avoiding float comparison).
	if approvingVals*3 >= totalActive*2 {
		return &UpgradeQuorum{
			Approved:        true,
			ApprovingVals:   approvingVals,
			TotalActive:     totalActive,
			NeededForQuorum: 0,
		}, nil
	}

	// Ceiling division: needed = ceil(totalActive*2/3) - approvingVals
	neededForQuorum := (totalActive*2+2)/3 - approvingVals

	return &UpgradeQuorum{
		Approved:        false,
		ApprovingVals:   approvingVals,
		TotalActive:     totalActive,
		NeededForQuorum: neededForQuorum,
	}, nil
}

// UpdateActiveNodeVersions updates the active node versions to the given version
func UpdateActiveNodeVersions(
	ctx cosmos.Context,
	thornadoKeeper keeper.Keeper,
	version string,
) error {
	activeVals, err := thornadoKeeper.ListActiveNodes(ctx)
	if err != nil {
		return fmt.Errorf("fail to get active nodes: %w", err)
	}

	for _, v := range activeVals {
		v.Version = version
		if err = thornadoKeeper.SetNodeAccount(ctx, v); err != nil {
			return fmt.Errorf("fail to save node account: %w", err)
		}
		ctx.EventManager().EmitEvent(
			cosmos.NewEvent("set_version",
				cosmos.NewAttribute("node_address", v.NodeAddress.String()),
				cosmos.NewAttribute("version", version)))
	}

	// update min join version to the fork version
	thornadoKeeper.SetMinJoinLast(ctx)

	return nil
}
