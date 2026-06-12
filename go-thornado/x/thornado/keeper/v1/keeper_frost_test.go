package keeperv1

import (
	. "gopkg.in/check.v1"
)

type KeeperFrostSuite struct{}

var _ = Suite(&KeeperFrostSuite{})

func (s *KeeperFrostSuite) TestFrostVoter(c *C) {
	ctx, k := setupKeeperForTest(c)

	pk := GetRandomPubKey()
	voter := NewFrostVoter("hello", nil, pk, GetRandomPubKey())

	v, err1 := k.GetFrostVoter(ctx, voter.ID)
	c.Check(err1, IsNil)
	c.Check(v.IsEmpty(), Equals, true)

	k.SetFrostVoter(ctx, voter)
	voter, err := k.GetFrostVoter(ctx, voter.ID)
	c.Assert(err, IsNil)
	c.Check(voter.ID, Equals, "hello")
	c.Check(voter.VaultPubKey.Equals(pk), Equals, true)
	iter := k.GetFrostVoterIterator(ctx)
	c.Check(iter, NotNil)
	iter.Close()
}

func (s *KeeperFrostSuite) TestFrostKeygenMetric(c *C) {
	ctx, k := setupKeeperForTest(c)
	pk := GetRandomPubKey()
	metric, err := k.GetFrostKeygenMetric(ctx, pk)
	c.Assert(err, IsNil)
	c.Assert(metric, NotNil)
	metric.AddNodeFrostTime(GetRandomBech32Addr(), 1024)
	k.SetFrostKeygenMetric(ctx, metric)

	metric1, err := k.GetFrostKeygenMetric(ctx, pk)
	c.Assert(err, IsNil)
	c.Assert(metric1, NotNil)
	c.Assert(metric1.NodeFrostTimes, HasLen, 1)
}

func (s *KeeperFrostSuite) TestFrostKeysignMetric(c *C) {
	ctx, k := setupKeeperForTest(c)
	txID := GetRandomTxHash()
	metric, err := k.GetFrostKeysignMetric(ctx, txID)
	c.Assert(err, IsNil)
	c.Assert(metric, NotNil)
	metric.AddNodeFrostTime(GetRandomBech32Addr(), 1024)
	k.SetFrostKeysignMetric(ctx, metric)

	metric1, err := k.GetFrostKeysignMetric(ctx, txID)
	c.Assert(err, IsNil)
	c.Assert(metric1, NotNil)
	c.Assert(metric1.NodeFrostTimes, HasLen, 1)
}
