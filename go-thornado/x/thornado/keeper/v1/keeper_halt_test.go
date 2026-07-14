package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
)

type KeeperHaltSuite struct{}

var _ = Suite(&KeeperHaltSuite{})

func (s *KeeperHaltSuite) TestUnmatchedOutboundRecord(c *C) {
	ctx, k := setupKeeperForTest(c)

	txID := common.TxID("9C77A11BF3DC3906BB3041DCAEC2B7FADEBF1BB0301984DB51A761834BE5F3BB")
	c.Check(k.GetUnmatchedOutboundHeight(ctx, txID), Equals, int64(0))

	k.SetUnmatchedOutboundHeight(ctx, txID, 144470)
	c.Check(k.GetUnmatchedOutboundHeight(ctx, txID), Equals, int64(144470))

	// txid case must not fork the record: Go-era observations post uppercase,
	// the Rust observer lowercase
	lower := common.TxID("9c77a11bf3dc3906bb3041dcaec2b7fadebf1bb0301984db51a761834be5f3bb")
	c.Check(k.GetUnmatchedOutboundHeight(ctx, lower), Equals, int64(144470))

	k.DeleteUnmatchedOutbound(ctx, lower)
	c.Check(k.GetUnmatchedOutboundHeight(ctx, txID), Equals, int64(0))
}
