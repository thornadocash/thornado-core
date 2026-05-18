package types

import (
	"sort"
	"strconv"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
)

type TypeTssSuite struct{}

var _ = Suite(&TypeTssSuite{})

func (s *TypeTssSuite) TestVoter(c *C) {
	pk := GetRandomPubKey()
	pks := []string{
		GetRandomPubKey().String(), GetRandomPubKey().String(), GetRandomPubKey().String(),
	}
	tss := NewTssVoter(
		"hello",
		pks,
		pk,
		GetRandomPubKey(),
	)
	c.Check(tss.IsEmpty(), Equals, false)
	c.Check(tss.String(), Equals, "hello")

	chains := []string{common.ETHChain.String(), common.BTCChain.String()}

	addr, err := common.PubKey(pks[0]).GetThorAddress()
	c.Assert(err, IsNil)
	c.Check(tss.HasSigned(addr), Equals, false)
	tss.Sign(addr, chains, "foo")
	c.Check(tss.Signers, HasLen, 1)
	c.Check(tss.HasSigned(addr), Equals, true)
	tss.Sign(addr, chains, "foo") // ensure signing twice doesn't duplicate
	c.Check(tss.Signers, HasLen, 1)
	c.Check(tss.Chains, HasLen, 2)
	c.Check(tss.Secp256K1Signatures, HasLen, 1)

	c.Check(tss.HasConsensus(), Equals, false)
	addr, err = common.PubKey(pks[1]).GetThorAddress()
	c.Assert(err, IsNil)
	tss.Sign(addr, chains, "")
	c.Check(tss.HasConsensus(), Equals, true)
	v1 := NewTssVoter("", nil, common.EmptyPubKey, common.EmptyPubKey)
	c.Check(v1.IsEmpty(), Equals, true)
}

func (s *TypeTssSuite) TestChainConsensus(c *C) {
	voter := TssVoter{
		PubKeys: []string{
			GetRandomPubKey().String(),
			GetRandomPubKey().String(),
			GetRandomPubKey().String(),
			GetRandomPubKey().String(),
		},
		Chains: []string{
			common.ETHChain.String(), // 4 ETH chains
			common.ETHChain.String(),
			common.ETHChain.String(),
			common.ETHChain.String(),
			common.BTCChain.String(), // 3 BTC chains
			common.BTCChain.String(),
			common.BTCChain.String(),
			common.GAIAChain.String(), // 2 GAIA chains
			common.GAIAChain.String(),
			common.THORChain.String(), // 1 THOR chain and partridge in a pear tree
		},
	}

	chains := voter.ConsensusChains()
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].String() < chains[j].String()
	})
	c.Check(chains, DeepEquals, common.Chains{common.BTCChain, common.ETHChain})
}

func (s *TypeTssSuite) TestConsensusCheckSignature(c *C) {
	pk := GetRandomPubKey()
	members := []string{
		GetRandomPubKey().String(), GetRandomPubKey().String(), GetRandomPubKey().String(),
	}

	// 3/3 post signature
	tss := NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for _, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)
		tss.Sign(addr, []string{common.ETHChain.String()}, "foo")
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok := tss.ConsensusCheckSignature()
	c.Check(ok, Equals, true)
	c.Check(sig, Equals, "foo")

	// 2/3 post signature
	tss = NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for i, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		_, ok = tss.ConsensusCheckSignature()
		c.Check(ok, Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)

		signature := ""
		if i > 0 {
			signature = "foo"
		}

		tss.Sign(addr, []string{common.ETHChain.String()}, signature)
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok = tss.ConsensusCheckSignature()
	c.Check(ok, Equals, true)
	c.Check(sig, Equals, "foo")

	// 1/3 posts signature
	tss = NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for i, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		_, ok = tss.ConsensusCheckSignature()
		c.Check(ok, Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)

		signature := ""
		if i == 0 {
			signature = "foo"
		}

		tss.Sign(addr, []string{common.ETHChain.String()}, signature)
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok = tss.ConsensusCheckSignature()
	c.Check(ok, Equals, false)
	c.Check(sig, Equals, "")

	// 1/3 posts different signature
	tss = NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for i, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		_, ok = tss.ConsensusCheckSignature()
		c.Check(ok, Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)

		signature := "foo"
		if i == 0 {
			signature = "bar"
		}

		tss.Sign(addr, []string{common.ETHChain.String()}, signature)
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok = tss.ConsensusCheckSignature()
	c.Check(ok, Equals, true)
	c.Check(sig, Equals, "foo")

	// no signatures posted
	tss = NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for _, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		_, ok = tss.ConsensusCheckSignature()
		c.Check(ok, Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)
		tss.Sign(addr, []string{common.ETHChain.String()}, "")
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok = tss.ConsensusCheckSignature()
	c.Check(ok, Equals, false)
	c.Check(sig, Equals, "")

	// all different signatures
	tss = NewTssVoter("foo", members, pk, pk)
	c.Check(tss.HasConsensus(), Equals, false)
	for i, member := range members {
		c.Check(tss.HasCompleteConsensus(), Equals, false)
		_, ok = tss.ConsensusCheckSignature()
		c.Check(ok, Equals, false)
		addr, err := common.PubKey(member).GetThorAddress()
		c.Assert(err, IsNil)
		tss.Sign(addr, []string{common.ETHChain.String()}, strconv.Itoa(i))
	}
	c.Check(tss.HasCompleteConsensus(), Equals, true)
	sig, ok = tss.ConsensusCheckSignature()
	c.Check(ok, Equals, false)
	c.Check(sig, Equals, "")
}
