//go:build !testnet && !mocknet
// +build !testnet,!mocknet

package utxo

import (
	. "gopkg.in/check.v1"
)

func (s *ZcashSignerSuite) TestGetChainCfg(c *C) {
	param := s.client.getChainCfgZEC()
	c.Assert(param.Name, Equals, "mainnet")
}
