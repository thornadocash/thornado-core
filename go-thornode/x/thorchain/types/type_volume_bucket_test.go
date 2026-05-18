package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type VolumeBucketSuite struct{}

var _ = Suite(&VolumeBucketSuite{})

func (s VolumeBucketSuite) TestVolumeBucket(c *C) {
	bucket := NewVolumeBucket(common.BTCAsset, 0)
	c.Assert(bucket.Valid(), IsNil)

	str := "asset:BTC.BTC index:0 amount-rune:0 amount-asset:0"
	c.Assert(bucket.String(), Equals, str)

	bucket.Asset = common.EmptyAsset
	c.Assert(bucket.Valid(), NotNil)

	bucket = NewVolumeBucket(common.BTCAsset, 0)
	bucket.AmountAsset = cosmos.Uint{}
	c.Assert(bucket.Valid(), NotNil)

	bucket = NewVolumeBucket(common.BTCAsset, 0)
	bucket.AmountRune = cosmos.Uint{}
	c.Assert(bucket.Valid(), NotNil)

	bucket = NewVolumeBucket(common.ETHAsset, -1)
	c.Assert(bucket.Valid(), NotNil)
}

func (s VolumeBucketSuite) TestEquals(c *C) {
	bucket1 := NewVolumeBucket(common.BTCAsset, 0)
	bucket2 := bucket1
	c.Assert(bucket1.Equals(bucket2), Equals, true)
	c.Assert(bucket2.Equals(NewVolumeBucket(common.BTCAsset, 0)), Equals, true)

	bucket1.Asset = common.ETHAsset
	c.Assert(bucket1.Equals(bucket2), Equals, false)

	bucket2.Asset = common.ETHAsset
	c.Assert(bucket1.Equals(bucket2), Equals, true)

	bucket1.AmountAsset = cosmos.NewUint(1)
	bucket1.AmountRune = cosmos.NewUint(2)
	c.Assert(bucket1.Equals(bucket2), Equals, false)

	bucket1.AmountAsset = cosmos.NewUint(0)
	c.Assert(bucket1.Equals(bucket2), Equals, false)

	bucket2.AmountRune = cosmos.NewUint(2)
	c.Assert(bucket1.Equals(bucket2), Equals, true)

	bucket1.Index = 8
	c.Assert(bucket1.Equals(bucket2), Equals, false)
}
