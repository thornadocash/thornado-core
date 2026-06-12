package keeperv1

import (
	. "gopkg.in/check.v1"
)

type KeeperFrostKeysignFailureSuite struct{}

var _ = Suite(&KeeperFrostKeysignFailureSuite{})

func (KeeperFrostKeysignFailureSuite) TestFrostKeysignFailVoter(c *C) {
	ctx, k := setupKeeperForTest(c)
	id := GetRandomTxHash().String()
	voter, err := k.GetFrostKeysignFailVoter(ctx, id)
	c.Check(err, IsNil)
	c.Check(voter.Empty(), Equals, true)

	k.SetFrostKeysignFailVoter(ctx, NewFrostKeysignFailVoter(id, 1024))
	voter1, err1 := k.GetFrostKeysignFailVoter(ctx, id)
	c.Check(err1, IsNil)
	c.Check(voter1.Empty(), Equals, false)
	c.Check(k.GetFrostKeysignFailVoterIterator(ctx), NotNil)
}
