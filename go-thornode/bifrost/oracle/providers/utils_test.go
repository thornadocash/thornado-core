package providers

import (
	"math/big"

	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestCheckFloat(c *C) {
	c.Assert(checkFloat(big.NewFloat(4621343.234)), IsNil)
	c.Assert(checkFloat(big.NewFloat(0)), NotNil)
	c.Assert(checkFloat(big.NewFloat(-4621343.234)), NotNil)
	c.Assert(checkFloat(nil), NotNil)
}
