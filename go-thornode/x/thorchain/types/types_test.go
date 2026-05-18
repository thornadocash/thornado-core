package types

import (
	"testing"

	. "gopkg.in/check.v1"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func TestPackage(t *testing.T) { TestingT(t) }

type TypesSuite struct{}

var _ = Suite(&TypesSuite{})

var (
	ethSingleTxFee = cosmos.NewUint(37500)
	ethMultiTxFee  = cosmos.NewUint(30000)
)

// Gas Fees
var ETHGasFeeSingleton = common.Gas{
	{Asset: common.ETHAsset, Amount: ethSingleTxFee},
}

var ETHGasFeeMulti = common.Gas{
	{Asset: common.ETHAsset, Amount: ethMultiTxFee},
}

func (s TypesSuite) TestHasSuperMajority(c *C) {
	// happy path
	c.Check(HasSuperMajority(3, 4), Equals, true)
	c.Check(HasSuperMajority(2, 3), Equals, true)
	c.Check(HasSuperMajority(4, 4), Equals, true)
	c.Check(HasSuperMajority(1, 1), Equals, true)
	c.Check(HasSuperMajority(67, 100), Equals, true)

	// unhappy path
	c.Check(HasSuperMajority(2, 4), Equals, false)
	c.Check(HasSuperMajority(9, 4), Equals, false)
	c.Check(HasSuperMajority(-9, 4), Equals, false)
	c.Check(HasSuperMajority(9, -4), Equals, false)
	c.Check(HasSuperMajority(0, 0), Equals, false)
	c.Check(HasSuperMajority(3, 0), Equals, false)
	c.Check(HasSuperMajority(8, 15), Equals, false)
}

func (TypesSuite) TestHasSimpleMajority(c *C) {
	c.Check(HasSimpleMajority(3, 4), Equals, true)
	c.Check(HasSimpleMajority(2, 3), Equals, true)
	c.Check(HasSimpleMajority(1, 2), Equals, true)
	c.Check(HasSimpleMajority(1, 3), Equals, false)
	c.Check(HasSimpleMajority(2, 4), Equals, true)
	c.Check(HasSimpleMajority(100000, 3000000), Equals, false)
}

func (TypesSuite) TestHasMinority(c *C) {
	c.Check(HasMinority(3, 4), Equals, true)
	c.Check(HasMinority(2, 3), Equals, true)
	c.Check(HasMinority(1, 2), Equals, true)
	c.Check(HasMinority(1, 3), Equals, true)
	c.Check(HasMinority(2, 4), Equals, true)
	c.Check(HasMinority(1, 4), Equals, false)
	c.Check(HasMinority(100000, 3000000), Equals, false)
}

func EnsureMsgBasicCorrect(m cosmos.Msg, c *C) {
	legacyMsg, ok := m.(sdk.LegacyMsg)
	c.Check(ok, Equals, true)
	signers := legacyMsg.GetSigners()
	c.Check(signers, NotNil)
	c.Check(len(signers), Equals, 1)
	msgV, ok := m.(sdk.HasValidateBasic)
	c.Check(ok, Equals, true)
	c.Check(msgV.ValidateBasic(), IsNil)
}
