package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type VolumeSuite struct{}

var _ = Suite(&VolumeSuite{})

func (s VolumeSuite) TestVolume(c *C) {
	volume := NewVolume(common.BTCAsset)
	c.Assert(volume.Valid(), IsNil)

	str := "asset:BTC.BTC total-rune:0 total-asset:0 change-rune:0 change-asset:0 last-bucket:-1"
	c.Assert(volume.String(), Equals, str)

	volume.Asset = common.EmptyAsset
	c.Assert(volume.Valid(), NotNil)

	volume = NewVolume(common.BTCAsset)
	volume.ChangeAsset = cosmos.Uint{}
	c.Assert(volume.Valid(), NotNil)

	volume = NewVolume(common.BTCAsset)
	volume.ChangeRune = cosmos.Uint{}
	c.Assert(volume.Valid(), NotNil)

	volume = NewVolume(common.BTCAsset)
	volume.TotalAsset = cosmos.Uint{}
	c.Assert(volume.Valid(), NotNil)

	volume = NewVolume(common.BTCAsset)
	volume.TotalRune = cosmos.Uint{}
	c.Assert(volume.Valid(), NotNil)
}

func (s VolumeSuite) TestEquals(c *C) {
	volume1 := NewVolume(common.BTCAsset)
	volume2 := volume1
	c.Assert(volume1.Asset.Equals(common.BTCAsset), Equals, true)
	c.Assert(volume1.Equals(volume2), Equals, true)

	volume2.Asset = common.ETHAsset
	c.Assert(volume1.Equals(volume2), Equals, false)

	volume2.Asset = common.BTCAsset
	c.Assert(volume1.Equals(volume2), Equals, true)

	volume2.LastBucket = 4
	c.Assert(volume1.Equals(volume2), Equals, false)

	volume2 = NewVolume(common.BTCAsset)
	c.Assert(volume1.Equals(volume2), Equals, true)

	volume2.ChangeRune = cosmos.NewUint(1)
	c.Assert(volume1.Equals(volume2), Equals, false)

	volume2 = NewVolume(common.BTCAsset)
	volume2.ChangeAsset = cosmos.NewUint(2)
	c.Assert(volume1.Equals(volume2), Equals, false)

	volume2 = NewVolume(common.BTCAsset)
	volume2.TotalRune = cosmos.NewUint(3)
	c.Assert(volume1.Equals(volume2), Equals, false)

	volume2 = NewVolume(common.BTCAsset)
	volume2.TotalAsset = cosmos.NewUint(4)
	c.Assert(volume1.Equals(volume2), Equals, false)
}
