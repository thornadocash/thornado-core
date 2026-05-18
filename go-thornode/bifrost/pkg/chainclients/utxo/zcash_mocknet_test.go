//go:build mocknet
// +build mocknet

package utxo

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
)

func (s *ZcashSuite) TestGetAddress(c *C) {
	pubkey := common.PubKey("tthorpub1zcjduepqrcthx0ke3r2z39rp42xrr777af7qfcs6wcxtxck6tj9j0ap8cl0q0msnrn")
	addr := s.client.GetAddress(pubkey)
	c.Assert(addr, Equals, "tmAbHS91acTju5QaDawZ1M5Pkbv2FFnRwUX")
}
