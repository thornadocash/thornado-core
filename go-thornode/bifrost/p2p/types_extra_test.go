package p2p

import (
	. "gopkg.in/check.v1"
)

type AddrListExtraTestSuite struct{}

var _ = Suite(&AddrListExtraTestSuite{})

func (AddrListExtraTestSuite) TestAddrListSetInvalidAddr(c *C) {
	al := addrList{}
	err := al.Set("not_a_valid_multiaddr")
	c.Assert(err, NotNil)
}

func (AddrListExtraTestSuite) TestAddrListStringEmpty(c *C) {
	al := addrList{}
	c.Assert(al.String(), Equals, "")
}

func (AddrListExtraTestSuite) TestAddrListMultiple(c *C) {
	al := addrList{}
	c.Assert(al.Set("/ip4/127.0.0.1/tcp/1234"), IsNil)
	c.Assert(al.Set("/ip4/127.0.0.1/tcp/5678"), IsNil)
	c.Assert(al.String(), Equals, "/ip4/127.0.0.1/tcp/1234,/ip4/127.0.0.1/tcp/5678")
}

func (AddrListExtraTestSuite) TestConfigMethods(c *C) {
	cfg := &Config{
		RendezvousString: "test-rendezvous",
		Port:             1234,
		ExternalIP:       "10.0.0.1",
	}

	c.Assert(cfg.GetRendezvous(), Equals, "test-rendezvous")
	c.Assert(cfg.GetP2PPort(), Equals, 1234)
	c.Assert(cfg.GetExternalIP(), Equals, "10.0.0.1")

	peers, err := cfg.GetBootstrapPeers()
	c.Assert(err, IsNil)
	c.Assert(peers, IsNil)
}
