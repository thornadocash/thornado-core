package keeperv1

import (
	. "gopkg.in/check.v1"

	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type KeeperRUNEProviderSuite struct{}

var _ = Suite(&KeeperRUNEProviderSuite{})

func (mas *KeeperRUNEProviderSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (s *KeeperRUNEProviderSuite) TestRUNEProvider(c *C) {
	ctx, k := setupKeeperForTest(c)

	addr := GetRandomRUNEAddress()
	accAddr, err := addr.AccAddress()
	c.Check(err, IsNil)
	rp, err := k.GetRUNEProvider(ctx, accAddr)
	c.Assert(err, IsNil)
	c.Check(rp.RuneAddress, NotNil)
	c.Check(rp.Units, NotNil)

	addr = GetRandomRUNEAddress()
	accAddr, err = addr.AccAddress()
	c.Assert(err, IsNil)
	rp = RUNEProvider{
		Units:         cosmos.NewUint(12),
		DepositAmount: cosmos.NewUint(12),
		RuneAddress:   accAddr,
	}
	k.SetRUNEProvider(ctx, rp)
	rp, err = k.GetRUNEProvider(ctx, rp.RuneAddress)
	c.Assert(err, IsNil)
	c.Check(rp.RuneAddress.Equals(accAddr), Equals, true)
	c.Check(rp.Units.Equal(cosmos.NewUint(12)), Equals, true)
	c.Check(rp.DepositAmount.Equal(cosmos.NewUint(12)), Equals, true)
	c.Check(rp.WithdrawAmount.Equal(cosmos.NewUint(0)), Equals, true)

	var rps []RUNEProvider
	iterator := k.GetRUNEProviderIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		k.Cdc().MustUnmarshal(iterator.Value(), &rp)
		if rp.RuneAddress.Empty() {
			continue
		}
		rps = append(rps, rp)
	}
	c.Check(rps[0].RuneAddress.Equals(accAddr), Equals, true)

	secondAddr := GetRandomRUNEAddress()
	secondAccAddr, err := secondAddr.AccAddress()
	c.Check(err, IsNil)
	rp2 := RUNEProvider{
		Units:         cosmos.NewUint(24),
		DepositAmount: cosmos.NewUint(24),
		RuneAddress:   secondAccAddr,
	}
	k.SetRUNEProvider(ctx, rp2)

	rps = []RUNEProvider{}
	iterator = k.GetRUNEProviderIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		k.Cdc().MustUnmarshal(iterator.Value(), &rp)
		if rp.RuneAddress.Empty() {
			continue
		}
		rps = append(rps, rp)
	}
	c.Check(len(rps), Equals, 2)

	totalUnits := cosmos.ZeroUint()
	iterator = k.GetRUNEProviderIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		k.Cdc().MustUnmarshal(iterator.Value(), &rp)
		if rp.RuneAddress.Empty() {
			continue
		}
		totalUnits = totalUnits.Add(rp.Units)
	}
	c.Check(totalUnits.Equal(cosmos.NewUint(36)), Equals, true)

	k.RemoveRUNEProvider(ctx, rp)
	k.RemoveRUNEProvider(ctx, rp2)
}
