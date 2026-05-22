package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

func (KeeperTestSuit) TestShielderState(c *C) {
	ctx, k := setupKeeperForTest(c)
	owner := GetRandomBech32Addr()
	pubkey := GetRandomPubKey()
	addr := GetRandomBTCAddress()
	txid := GetRandomTxHash()

	session := types.ShielderSession{
		Owner:          owner,
		PowToken:       "pow-token",
		DepositAddress: addr,
		VaultPubKey:    pubkey,
		CreatedHeight:  ctx.BlockHeight(),
		Status:         types.ShielderStatusAddressIssued,
	}
	c.Assert(k.SetShielderSession(ctx, session), IsNil)

	gotSession, err := k.GetShielderSession(ctx, owner)
	c.Assert(err, IsNil)
	c.Check(gotSession.PowToken, Equals, "pow-token")

	gotByPow, err := k.GetShielderSessionByPowToken(ctx, "pow-token")
	c.Assert(err, IsNil)
	c.Check(gotByPow.Owner.String(), Equals, owner.String())

	deposit := types.ShielderDeposit{
		DepositID:      txid,
		Owner:          owner,
		AmountSats:     100_000,
		DepositAddress: addr,
		VaultPubKey:    pubkey,
		MatchedHeight:  ctx.BlockHeight(),
		Status:         types.ShielderStatusDepositMatched,
	}
	c.Assert(k.SetShielderDeposit(ctx, deposit), IsNil)
	gotDeposit, err := k.GetShielderDeposit(ctx, txid)
	c.Assert(err, IsNil)
	c.Check(gotDeposit.AmountSats, Equals, uint64(100_000))

	c.Assert(k.SetShielderCommitment(ctx, "commitment-1", txid), IsNil)
	c.Check(k.ShielderCommitmentExists(ctx, "commitment-1"), Equals, true)

	withdrawalID := "ABCDEF"
	withdrawal := types.ShielderWithdrawal{
		WithdrawalID:    withdrawalID,
		Owner:           owner,
		NullifierHash:   "nullifier",
		MerkleRoot:      "root",
		Recipient:       addr,
		AmountSats:      50_000,
		FeeSats:         1_000,
		InHash:          common.BlankTxID,
		VaultPubKey:     pubkey,
		RequestedHeight: ctx.BlockHeight(),
		Status:          types.ShielderStatusKeysignQueued,
	}
	c.Assert(k.SetShielderWithdrawal(ctx, withdrawal), IsNil)
	gotWithdrawal, err := k.GetShielderWithdrawal(ctx, withdrawalID)
	c.Assert(err, IsNil)
	c.Check(gotWithdrawal.NullifierHash, Equals, "nullifier")

	c.Check(k.ShielderNullifierSpent(ctx, "nullifier"), Equals, false)
	c.Assert(k.SetShielderNullifierSpent(ctx, "nullifier", withdrawalID), IsNil)
	c.Check(k.ShielderNullifierSpent(ctx, "nullifier"), Equals, true)

	slot, err := k.AllocateShielderNodeBondSlot(ctx)
	c.Assert(err, IsNil)
	c.Check(slot, Equals, uint64(0))
	nextSlot, err := k.AllocateShielderNodeBondSlot(ctx)
	c.Assert(err, IsNil)
	c.Check(nextSlot, Equals, uint64(1))

	bond := types.ShielderValidatorBond{
		ValidatorPubKey: "validator-key",
		OperatorPubKey:  pubkey,
		NodeAddress:     owner,
		Slot:            slot,
		PendingSats:     50_000_000,
		BondSats:        100_000_000,
		CreatedHeight:   ctx.BlockHeight(),
		UpdatedHeight:   ctx.BlockHeight(),
	}
	c.Assert(k.SetShielderValidatorBond(ctx, bond), IsNil)
	gotBond, err := k.GetShielderValidatorBond(ctx, bond.ValidatorPubKey)
	c.Assert(err, IsNil)
	c.Check(gotBond.Slot, Equals, slot)
	c.Check(gotBond.PendingSats, Equals, uint64(50_000_000))
	c.Check(gotBond.BondSats, Equals, uint64(100_000_000))

	feePool := types.ShielderFeePool{
		PendingSats:        2_000_000,
		TotalSlots:         1,
		FeePerSlotShare:    20_000_000_000,
		TotalCollectedSats: 2_000_000,
	}
	c.Assert(k.SetShielderFeePool(ctx, feePool), IsNil)
	gotFeePool, err := k.GetShielderFeePool(ctx)
	c.Assert(err, IsNil)
	c.Check(gotFeePool.TotalCollectedSats, Equals, uint64(2_000_000))
	c.Check(gotFeePool.FeePerSlotShare, Equals, uint64(20_000_000_000))

	empty := types.ShielderWithdrawal{Owner: owner, AmountSats: 1, FeeSats: 0}
	c.Assert(empty.Valid(), NotNil)
	c.Assert(types.ShielderDeposit{Owner: owner, AmountSats: 1}.Valid(), NotNil)
	c.Assert(types.ShielderSession{Owner: cosmos.AccAddress{}}.Valid(), NotNil)
}
