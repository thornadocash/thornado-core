package common

import (
	"math/big"

	. "gopkg.in/check.v1"
)

type DecimalsSuite struct{}

var _ = Suite(&DecimalsSuite{})

func (s *DecimalsSuite) TestConversion(c *C) {
	amount1 := big.NewInt(9_999_999)
	amount2 := big.NewInt(1_234_567_890)

	// 1.2 trillion with 18 decimals: 1,200,300,400,500.100…
	amount3, ok := new(big.Int).SetString("1200300400500100100100100100123", 10)
	c.Assert(ok, Equals, true)

	result, err := ConvertDecimals(amount1, 6, 3)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "9999")

	result, err = ConvertDecimals(amount2, 9, 7)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "12345678")

	result, err = ConvertDecimals(amount3, 18, THORChainDecimals)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "120030040050010010010")

	result, err = ConvertDecimals(amount2, 6, 1)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "12345")

	result, err = ConvertDecimals(amount3, 18, 0)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "1200300400500")

	result, err = ConvertDecimals(amount1, 4, -1)
	c.Assert(err, Not(IsNil))
	c.Assert(result, IsNil)

	result, err = ConvertDecimals(amount1, -1, -1)
	c.Assert(err, Not(IsNil))
	c.Assert(result, IsNil)

	result, err = ConvertDecimals(amount1, -1, 10)
	c.Assert(err, Not(IsNil))
	c.Assert(result, IsNil)

	result, err = ConvertDecimals(amount1, 0, 6)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "9999999000000")

	result, err = ConvertDecimals(amount2, 6, 18)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, "1234567890000000000000")

	result, err = ConvertDecimals(amount1, 6, 6)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, amount1.String())

	result, err = ConvertDecimals(amount3, 18, 18)
	c.Assert(err, IsNil)
	c.Assert(result.String(), Equals, amount3.String())
}
