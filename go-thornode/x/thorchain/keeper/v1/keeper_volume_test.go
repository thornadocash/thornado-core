package keeperv1

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
	. "gopkg.in/check.v1"
)

type KeeperVolumeSuite struct{}

var _ = Suite(&KeeperVolumeSuite{})

func (s KeeperVolumeSuite) TestVolume(c *C) {
	ctx, k := setupKeeperForTest(c)

	volume := types.NewVolume(common.EmptyAsset)
	err := k.SetVolume(ctx, volume)
	c.Assert(err, NotNil)

	volume.Asset = common.BTCAsset
	err = k.SetVolume(ctx, volume)
	c.Assert(err, IsNil)

	_, err = k.GetVolume(ctx, common.BTCAsset)
	c.Assert(err, IsNil)

	_, err = k.GetVolume(ctx, common.ETHAsset)
	c.Assert(err, NotNil)
}

func (s KeeperVolumeSuite) TestVolumeBucket(c *C) {
	ctx, k := setupKeeperForTest(c)

	bucket := types.NewVolumeBucket(common.BTCAsset, -1)

	err := k.SetVolumeBucket(ctx, bucket)
	c.Assert(err, NotNil)

	bucket.Index = 0

	err = k.SetVolumeBucket(ctx, bucket)
	c.Assert(err, IsNil)

	_, err = k.GetVolumeBucket(ctx, common.BTCAsset, 0)
	c.Assert(err, IsNil)

	_, err = k.GetVolumeBucket(ctx, common.ETHAsset, 0)
	c.Assert(err, NotNil)
}
