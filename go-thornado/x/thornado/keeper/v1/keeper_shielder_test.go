package keeperv1

import (
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
		CreatedHeight:  ctx.BlockHeight(),
		UpdatedHeight:  ctx.BlockHeight(),
	}
	c.Assert(k.SetNodeAccount(ctx, NewNodeAccount(owner, NodeStandby, common.EmptyPubKeySet, bond.NodePubKey, cosmos.NewUint(bond.BondSats), bondAddress, ctx.BlockHeight())), IsNil)
	c.Assert(k.SetShielderNodeBond(ctx, bond), IsNil)
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
