package thorchain

import (
	"testing"

	"github.com/blang/semver"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type MemoMaintSuite struct{}

var _ = Suite(&MemoMaintSuite{})

func TestMemoMaintSuite(t *testing.T) {
	TestingT(t)
}

func (s *MemoMaintSuite) TestMaintMemo(c *C) {
	addr := types.GetRandomBech32Addr()
	memo := NewMaintMemo(addr)

	// Test GetAccAddress
	c.Assert(memo.GetAccAddress(), DeepEquals, addr)

	// Test String method
	c.Assert(memo.String(), Equals, "maint:"+addr.String())

	// Create parser and test ParseMaintMemo
	parsedMemo, err := ParseMemo(semver.MustParse("1.0.0"), memo.String())
	c.Assert(err, IsNil)
	c.Assert(parsedMemo.GetType(), Equals, TxMaint)

	maintMemo, ok := parsedMemo.(MaintMemo)
	c.Assert(ok, Equals, true)
	c.Assert(maintMemo.NodeAddress.String(), Equals, addr.String())
}

func (s *MemoMaintSuite) TestParseInvalidMaintMemo(c *C) {
	// Test with invalid address
	_, err := ParseMemo(semver.MustParse("1.0.0"), "maint:invalid")
	c.Assert(err, NotNil)

	// Test with missing address
	_, err = ParseMemo(semver.MustParse("1.0.0"), "maint:")
	c.Assert(err, NotNil)

	// Test with no parameters
	_, err = ParseMemo(semver.MustParse("1.0.0"), "maint")
	c.Assert(err, NotNil)
}
