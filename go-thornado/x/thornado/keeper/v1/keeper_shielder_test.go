package keeperv1

import (
	"encoding/json"
	"fmt"
	"sort"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func (KeeperTestSuit) TestShielderState(c *C) {
	ctx, k := setupKeeperForTest(c)
	owner := GetRandomBech32Addr()
	pubkey := GetRandomPubKey()
	addr := GetRandomBTCAddress()
	txid := GetRandomTxHash()

	session := types.DepositSession{
		Owner:          owner,
		PowToken:       "pow-token",
		DepositAddress: addr,
		VaultPubKey:    pubkey,
		CreatedHeight:  ctx.BlockHeight(),
		Status:         types.DepositStatusAddressIssued,
	}
	c.Assert(k.SetDepositSession(ctx, session), IsNil)

	gotSession, err := k.GetDepositSession(ctx, owner)
	c.Assert(err, IsNil)
	c.Check(gotSession.PowToken, Equals, "pow-token")

	gotByPow, err := k.GetDepositSessionByPowToken(ctx, "pow-token")
	c.Assert(err, IsNil)
	c.Check(gotByPow.Owner.String(), Equals, owner.String())

	deposit := types.DepositRecord{
		DepositID:      txid,
		Owner:          owner,
		AmountSats:     100_000,
		DepositAddress: addr,
		VaultPubKey:    pubkey,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.DepositStatusDepositMatched,
	}
	c.Assert(k.SetDepositRecord(ctx, deposit), IsNil)
	gotDeposit, err := k.GetDepositRecord(ctx, txid)
	c.Assert(err, IsNil)
	c.Check(gotDeposit.AmountSats, Equals, uint64(100_000))

	c.Assert(k.SetShielderCommitment(ctx, "commitment-1"), IsNil)
	c.Check(k.ShielderCommitmentExists(ctx, "commitment-1"), Equals, true)

	withdrawalID := "ABCDEF"
	withdrawal := types.ShielderRedeem{
		WithdrawalID:    withdrawalID,
		NullifierHash:   "nullifier",
		MerkleRoot:      "root",
		Recipient:       addr,
		AmountSats:      50_000,
		FeeSats:         1_000,
		InHash:          common.BlankTxID,
		VaultPubKey:     pubkey,
		RequestedHeight: ctx.BlockHeight(),
		Status:          types.DepositStatusKeysignQueued,
	}
	c.Assert(k.SetShielderRedeem(ctx, withdrawal), IsNil)
	gotWithdrawal, err := k.GetShielderRedeem(ctx, withdrawalID)
	c.Assert(err, IsNil)
	c.Check(gotWithdrawal.NullifierHash, Equals, "nullifier")

	c.Check(k.ShielderNullifierSpent(ctx, "nullifier"), Equals, false)
	c.Assert(k.SetShielderNullifierSpent(ctx, "nullifier", withdrawalID), IsNil)
	c.Check(k.ShielderNullifierSpent(ctx, "nullifier"), Equals, true)

	slot, err := k.AllocateShielderNodeBondSlot(ctx)
	c.Assert(err, IsNil)
	c.Check(slot, Equals, uint64(1))
	nextSlot, err := k.AllocateShielderNodeBondSlot(ctx)
	c.Assert(err, IsNil)
	c.Check(nextSlot, Equals, uint64(2))

	bond := types.ShielderNodeBond{
		NodePubKey:     "node-key",
		OperatorPubKey: pubkey,
		NodeAddress:    owner,
		Slot:           slot,
		PendingSats:    50_000_000,
		BondSats:       100_000_000,
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	c.Assert(k.SetShielderNodeBond(ctx, bond), IsNil)
	gotBond, err := k.GetShielderNodeBond(ctx, bond.NodePubKey)
	c.Assert(err, IsNil)
	c.Check(gotBond.Slot, Equals, slot)
	c.Check(gotBond.PendingSats, Equals, uint64(50_000_000))
	c.Check(gotBond.BondSats, Equals, uint64(100_000_000))

	feePool := types.FeePool{
		PendingSats:        2_000_000,
		TotalSlots:         1,
		FeePerSlotShare:    20_000_000_000,
		TotalCollectedSats: 2_000_000,
	}
	c.Assert(k.SetFeePool(ctx, feePool), IsNil)
	gotFeePool, err := k.GetFeePool(ctx)
	c.Assert(err, IsNil)
	c.Check(gotFeePool.TotalCollectedSats, Equals, uint64(2_000_000))
	c.Check(gotFeePool.FeePerSlotShare, Equals, uint64(20_000_000_000))

	empty := types.ShielderRedeem{AmountSats: 1, FeeSats: 0}
	c.Assert(empty.Valid(), NotNil)
	c.Assert(types.DepositRecord{Owner: owner, AmountSats: 1}.Valid(), NotNil)
	c.Assert(types.DepositSession{Owner: cosmos.AccAddress{}}.Valid(), NotNil)
}

func (KeeperTestSuit) TestShielderInvariants(c *C) {
	ctx, k := setupKeeperForTest(c)
	owner := GetRandomBech32Addr()
	vaultPubKey := GetRandomPubKey()
	depositAddress := GetRandomBTCAddress()
	depositID := GetRandomTxHash()
	commitment := "COMMITMENT_A"

	deposit := types.DepositRecord{
		DepositID:      depositID,
		Owner:          owner,
		AmountSats:     100_000,
		DepositAddress: depositAddress,
		VaultPubKey:    vaultPubKey,
		Settlement:     types.DepositSettlementUser,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.DepositStatusCommitted,
	}
	c.Assert(k.SetDepositRecord(ctx, deposit), IsNil)
	c.Assert(k.SetShielderCommitment(ctx, commitment), IsNil)
	c.Assert(k.SetNextVaultDepositPathIndex(ctx, vaultPubKey, common.VaultDepositPathUser, 2), IsNil)
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	c.Assert(k.SetDepositAddress(ctx, types.DepositAddress{
		Address:       depositAddress,
		VaultPubKey:   vaultPubKey,
		PathIndex:     pathIndex,
		Path:          common.VaultDepositPath(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot),
		PathType:      string(common.VaultDepositPathUser),
		DepositNonce:  0,
		Owner:         owner,
		PowToken:      "pow-token",
		CreatedHeight: ctx.BlockHeight(),
	}), IsNil)
	vault := NewVaultV2(ctx.BlockHeight(), ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000_000))))
	c.Assert(k.SetVault(ctx, vault), IsNil)

	bondAddress, err := common.NewAddress(owner.String())
	c.Assert(err, IsNil)
	bond := types.ShielderNodeBond{
		NodePubKey:     "node-key",
		OperatorPubKey: vaultPubKey,
		NodeAddress:    owner,
		Slot:           0,
		BondSats:       100_000_000,
		FeeDebtSats:    0,
		FeeShareActive: true,
		Bonders:        []string{owner.String()},
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	c.Assert(k.SetNodeAccount(ctx, NewNodeAccount(owner, NodeStandby, common.EmptyPubKeySet, bond.NodePubKey, cosmos.NewUint(bond.BondSats), bondAddress, ctx.BlockHeight())), IsNil)
	c.Assert(k.SetShielderNodeBond(ctx, bond), IsNil)
	c.Assert(k.SetShielderNodeBonder(ctx, types.ShielderNodeBonder{
		NodePubKey:    bond.NodePubKey,
		Bonder:        owner,
		PrincipalSats: bond.BondSats,
		CreatedHeight: ctx.BlockHeight(),
		UpdatedHeight: ctx.BlockHeight(),
	}), IsNil)
	c.Assert(k.SetFeePool(ctx, types.FeePool{TotalSlots: 1}), IsNil)

	for _, route := range k.InvariantRoutes() {
		msg, broken := route.Invariant(ctx)
		c.Check(broken, Equals, false, Commentf("%s: %v", route.Route, msg))
	}

	c.Assert(k.SetNextVaultDepositPathIndex(ctx, vaultPubKey, common.VaultDepositPathUser, 0), IsNil)
	msg, broken := ShielderVaultAddressInvariant(k)(ctx)
	c.Check(broken, Equals, true)
	c.Check(len(msg) > 0, Equals, true)
}

func (KeeperTestSuit) TestVaultBackingInvariantDetectsDeficit(c *C) {
	ctx, k := setupKeeperForTest(c)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "NOTE_A",
		DenominationSats: 100_000,
	}), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, true)
	c.Check(len(msg) > 0, Equals, true)

	vault := NewVaultV2(ctx.BlockHeight(), ActiveVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))))
	c.Assert(k.SetVault(ctx, vault), IsNil)

	msg, broken = VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))
}

func (KeeperTestSuit) TestVaultBackingInvariantCountsDepositsAndInternalInflight(c *C) {
	ctx, k := setupKeeperForTest(c)
	owner := GetRandomBech32Addr()
	vaultPubKey := GetRandomPubKey()
	depositID := GetRandomTxHash()

	c.Assert(k.SetDepositRecord(ctx, types.DepositRecord{
		DepositID:        depositID,
		Owner:            owner,
		AmountSats:       100_000,
		ShieldedSats:     100_000,
		DepositAddress:   GetRandomBTCAddress(),
		VaultPubKey:      vaultPubKey,
		DepositPathIndex: 1,
		MatchedHeight:    ctx.BlockHeight(),
		Status:           types.DepositStatusCommitted,
	}), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "NOTE_SWEEP_INFLIGHT",
		DenominationSats: 100_000,
	}), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	c.Assert(k.SetTxOut(ctx, &TxOut{
		Height: ctx.BlockHeight(),
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000)),
				TxType:      types.TxOutTypeSweep,
				InHash:      depositID,
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: vaultPubKey,
				GasRate:     1,
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
			},
		},
	}), IsNil)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))
}

func (KeeperTestSuit) TestVaultBackingInvariantCountsIncompleteInternalOutHashAsInflight(c *C) {
	ctx, k := setupKeeperForTest(c)
	vaultPubKey := GetRandomPubKey()
	outHash := GetRandomTxHash()

	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "NOTE_INTERNAL_OUTHASH_INFLIGHT",
		DenominationSats: 100_000,
	}), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	c.Assert(k.SetTxOut(ctx, &TxOut{
		Height: ctx.BlockHeight(),
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000)),
				TxType:      types.TxOutTypeSweep,
				InHash:      GetRandomTxHash(),
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: vaultPubKey,
				OutHash:     outHash,
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
			},
		},
	}), IsNil)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))
}

func (KeeperTestSuit) TestVaultBackingInvariantCountsCompletedInternalOutHashAsControlled(c *C) {
	ctx, k := setupKeeperForTest(c)
	vaultPubKey := GetRandomPubKey()
	outHash := GetRandomTxHash()
	toAddress := GetRandomBTCAddress()

	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "NOTE_COMPLETED_INTERNAL",
		DenominationSats: 100_000,
	}), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	c.Assert(k.SetTxOut(ctx, &TxOut{
		Height: ctx.BlockHeight(),
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000)),
				TxType:      types.TxOutTypeMigrate,
				InHash:      GetRandomTxHash(),
				ToAddress:   toAddress,
				VaultPubKey: vaultPubKey,
				OutHash:     outHash,
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
				SourceInputs: []types.TxOutInput{
					{TxId: GetRandomTxHash(), Vout: 0, AmountSats: 100_000},
				},
			},
		},
	}), IsNil)
	tx := common.NewTx(
		outHash,
		GetRandomBTCAddress(),
		toAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
	)
	observed := common.NewObservedTx(tx, 1, vaultPubKey, 1)
	voter := NewObservedTxVoter(outHash, ObservedTxs{observed})
	voter.Tx = observed
	k.SetObservedTxOutVoter(ctx, voter)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))
}

func (KeeperTestSuit) TestVaultBackingInvariantCountsCompletedExternalGas(c *C) {
	ctx, k := setupKeeperForTest(c)
	vaultPubKey := GetRandomPubKey()
	outHash := GetRandomTxHash()

	vault := NewVaultV2(ctx.BlockHeight(), ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000))))
	c.Assert(k.SetVault(ctx, vault), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "NOTE_EXTERNAL_GAS",
		DenominationSats: 100_000,
	}), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	c.Assert(k.SetTxOut(ctx, &TxOut{
		Height: ctx.BlockHeight(),
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000)),
				TxType:      types.TxOutTypeOut,
				InHash:      GetRandomTxHash(),
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: vaultPubKey,
				OutHash:     outHash,
				OutVout:     0,
			},
		},
	}), IsNil)
	tx := common.NewTx(
		outHash,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
	)
	observed := common.NewObservedTx(tx, 1, vaultPubKey, 1)
	voter := NewObservedTxVoter(outHash, ObservedTxs{observed})
	voter.Tx = observed
	k.SetObservedTxOutVoter(ctx, voter)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))
}

func (KeeperTestSuit) TestSweepOrphanShielderNoteRecords(c *C) {
	ctx, k := setupKeeperForTest(c)
	denom := uint64(10_000_000)

	// Live tree: two leaves at indices 0 and 1.
	c.Assert(k.SetShielderDenominationLeaf(ctx, denom, 0, "aa11"), IsNil)
	c.Assert(k.SetShielderDenominationLeaf(ctx, denom, 1, "bb22"), IsNil)

	// Live records corroborated by the leaf store.
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "aa11", DenominationSats: denom, LeafIndex: 0, CreatedHeight: 10,
	}), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "bb22", DenominationSats: denom, LeafIndex: 1, CreatedHeight: 11,
	}), IsNil)
	// Orphans: index-0 records whose commitment is not leaf 0, and a record
	// pointing past the tree.
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "dead01", DenominationSats: denom, LeafIndex: 0, CreatedHeight: 3,
	}), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "dead02", DenominationSats: denom, LeafIndex: 0, CreatedHeight: 4,
	}), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "dead03", DenominationSats: denom, LeafIndex: 9, CreatedHeight: 5,
	}), IsNil)
	// A different denomination is untouched even with a bogus index.
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "cc33", DenominationSats: denom * 10, LeafIndex: 0, CreatedHeight: 6,
	}), IsNil)
	// Legacy pre-index layout: commitment-keyed bool entry under the
	// denomination prefix. It breaks GetShielderDenominationCommitments and
	// must be swept too.
	legacyKey := []byte(shielderDenominationPrefix(denom) + "FEEDLEGACY")
	c.Assert(k.setShielderJSON(ctx, legacyKey, true), IsNil)

	n, err := k.SweepOrphanShielderNoteRecords(ctx, denom)
	c.Assert(err, IsNil)
	c.Check(n, Equals, 4)

	// The typed accessor works again and sees exactly the live leaves.
	leaves, err := k.GetShielderDenominationCommitments(ctx, denom)
	c.Assert(err, IsNil)
	c.Check(leaves, DeepEquals, []string{"aa11", "bb22"})

	var remaining []string
	iter := k.GetShielderNoteRecordIterator(ctx)
	for ; iter.Valid(); iter.Next() {
		var record types.StoredShielderNoteRecord
		c.Assert(json.Unmarshal(iter.Value(), &record), IsNil)
		remaining = append(remaining, record.Commitment)
	}
	iter.Close()
	sort.Strings(remaining)
	c.Check(remaining, DeepEquals, []string{"aa11", "bb22", "cc33"})

	// Idempotent: nothing left to sweep.
	n, err = k.SweepOrphanShielderNoteRecords(ctx, denom)
	c.Assert(err, IsNil)
	c.Check(n, Equals, 0)
}

// fakeMerkleAppend is a deterministic stand-in for the Rust incremental append:
// each root chains on the previous one, so replayed roots are unique per prefix.
func fakeMerkleAppend(filled []string, index uint64, leaf string) (string, []string, error) {
	prev := "seed"
	if len(filled) > 0 {
		prev = filled[0]
	}
	root := fmt.Sprintf("r(%s+%s@%d)", prev, leaf, index)
	return root, []string{root}, nil
}

func (KeeperTestSuit) TestSweepOldRegimeShielderSpends(c *C) {
	ctx, k := setupKeeperForTest(c)
	denom := uint64(10_000_000)
	recipient := GetRandomBTCAddress()

	c.Assert(k.SetShielderDenominationLeaf(ctx, denom, 0, "aa11"), IsNil)
	c.Assert(k.SetShielderDenominationLeaf(ctx, denom, 1, "bb22"), IsNil)
	root0, _, err := fakeMerkleAppend(nil, 0, "aa11")
	c.Assert(err, IsNil)
	root1, _, err := fakeMerkleAppend([]string{root0}, 1, "bb22")
	c.Assert(err, IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: denom, NextIndex: 2, FilledSubtrees: []string{root1}, Root: root1,
	}), IsNil)

	c.Assert(k.SetShielderMerkleRoot(ctx, denom, root0), IsNil)
	c.Assert(k.SetShielderMerkleRoot(ctx, denom, root1), IsNil)
	c.Assert(k.SetShielderMerkleRoot(ctx, denom, "OLDROOT1"), IsNil)
	// A denomination with recorded roots but no tree state is entirely old-regime.
	c.Assert(k.SetShielderMerkleRoot(ctx, denom*10, "OLDROOT2"), IsNil)

	oldOutHash := GetRandomTxHash()
	c.Assert(k.SetShielderRedeem(ctx, types.ShielderRedeem{
		WithdrawalID: "WNEW", NullifierHash: "null1", MerkleRoot: root0,
		Recipient: recipient, AmountSats: denom, InHash: common.BlankTxID,
		VaultPubKey: GetRandomPubKey(), RequestedHeight: 5, Status: "settled",
	}), IsNil)
	c.Assert(k.SetShielderRedeem(ctx, types.ShielderRedeem{
		WithdrawalID: "WOLD", NullifierHash: "null2", MerkleRoot: "OLDROOT1",
		Recipient: recipient, AmountSats: denom, InHash: common.BlankTxID,
		OutHash: oldOutHash, VaultPubKey: GetRandomPubKey(), RequestedHeight: 3, Status: "settled",
	}), IsNil)
	c.Assert(k.SetShielderRedeemOutHash(ctx, oldOutHash.String(), "WOLD"), IsNil)
	c.Assert(k.SetShielderNullifierSpent(ctx, "null1", "WNEW"), IsNil)
	c.Assert(k.SetShielderNullifierSpent(ctx, "null2", "WOLD"), IsNil)
	// Nullifier whose redeem record is missing: era undecidable, must survive.
	c.Assert(k.SetShielderNullifierSpent(ctx, "null3", "WGONE"), IsNil)

	rootsDeleted, spendsDeleted, err := k.SweepOldRegimeShielderSpends(ctx, fakeMerkleAppend)
	c.Assert(err, IsNil)
	c.Check(rootsDeleted, Equals, 2)
	c.Check(spendsDeleted, Equals, 1)

	c.Check(k.ShielderMerkleRootExists(ctx, denom, root0), Equals, true)
	c.Check(k.ShielderMerkleRootExists(ctx, denom, root1), Equals, true)
	c.Check(k.ShielderMerkleRootExists(ctx, denom, "OLDROOT1"), Equals, false)
	c.Check(k.ShielderMerkleRootExists(ctx, denom*10, "OLDROOT2"), Equals, false)

	c.Check(k.ShielderNullifierSpent(ctx, "null1"), Equals, true)
	c.Check(k.ShielderNullifierSpent(ctx, "null2"), Equals, false)
	c.Check(k.ShielderNullifierSpent(ctx, "null3"), Equals, true)

	kept, err := k.GetShielderRedeem(ctx, "WNEW")
	c.Assert(err, IsNil)
	c.Check(kept.AmountSats, Equals, denom)
	gone, err := k.GetShielderRedeem(ctx, "WOLD")
	c.Assert(err, IsNil)
	c.Check(gone.AmountSats, Equals, uint64(0))
	_, found, err := k.GetShielderRedeemByOutHash(ctx, oldOutHash.String())
	c.Assert(err, IsNil)
	c.Check(found, Equals, false)

	// Idempotent.
	rootsDeleted, spendsDeleted, err = k.SweepOldRegimeShielderSpends(ctx, fakeMerkleAppend)
	c.Assert(err, IsNil)
	c.Check(rootsDeleted, Equals, 0)
	c.Check(spendsDeleted, Equals, 0)
}

func (KeeperTestSuit) TestSweepOldRegimeShielderSpendsAbortsOnReplayMismatch(c *C) {
	ctx, k := setupKeeperForTest(c)
	denom := uint64(10_000_000)

	c.Assert(k.SetShielderDenominationLeaf(ctx, denom, 0, "aa11"), IsNil)
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: denom, NextIndex: 1, Root: "NOT_THE_REPLAY_ROOT",
	}), IsNil)
	c.Assert(k.SetShielderMerkleRoot(ctx, denom, "OLDROOT1"), IsNil)

	_, _, err := k.SweepOldRegimeShielderSpends(ctx, fakeMerkleAppend)
	c.Assert(err, NotNil)
	c.Check(k.ShielderMerkleRootExists(ctx, denom, "OLDROOT1"), Equals, true)

	// A leaf count that disagrees with NextIndex also aborts.
	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: denom, NextIndex: 2, Root: "whatever",
	}), IsNil)
	_, _, err = k.SweepOldRegimeShielderSpends(ctx, fakeMerkleAppend)
	c.Assert(err, NotNil)
}

func (KeeperTestSuit) TestVaultBackingInvariantFlagsOrphanNoteRecords(c *C) {
	ctx, k := setupKeeperForTest(c)

	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	vault := NewVaultV2(ctx.BlockHeight(), ActiveVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000))))
	c.Assert(k.SetVault(ctx, vault), IsNil)
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "NOTE_LIVE", DenominationSats: 100_000, LeafIndex: 0,
	}), IsNil)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, false, Commentf("%v", msg))

	// A second record of the same denomination exceeds the tree's one leaf:
	// orphan pollution, the SHIELDERNOTESWEEP condition.
	c.Assert(k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment: "NOTE_ORPHAN", DenominationSats: 100_000, LeafIndex: 0,
	}), IsNil)
	msg, broken = VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, true)
	c.Check(len(msg) > 0, Equals, true)
}

func (KeeperTestSuit) TestVaultBackingInvariantSpentExceedsTreeMinted(c *C) {
	ctx, k := setupKeeperForTest(c)
	recipient := GetRandomBTCAddress()

	c.Assert(k.SetShielderTreeState(ctx, types.StoredShielderTreeState{
		DenominationSats: 100_000, NextIndex: 1, Root: "ROOT_A",
	}), IsNil)
	c.Assert(k.SetShielderRedeem(ctx, types.ShielderRedeem{
		WithdrawalID: "WBIG", NullifierHash: "nullbig", MerkleRoot: "ROOT_A",
		Recipient: recipient, AmountSats: 200_000, InHash: common.BlankTxID,
		VaultPubKey: GetRandomPubKey(), RequestedHeight: 3, Status: "settled",
	}), IsNil)
	c.Assert(k.SetShielderNullifierSpent(ctx, "nullbig", "WBIG"), IsNil)

	msg, broken := VaultBackingInvariant(k)(ctx)
	c.Check(broken, Equals, true)
	c.Check(len(msg) > 0, Equals, true)
}
