package keeperv1

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) GetAnchors(ctx cosmos.Context, asset common.Asset) []common.Asset {
	return []common.Asset{asset.GetLayer1Asset()}
}

func (k KVStore) AnchorMedian(ctx cosmos.Context, assets []common.Asset) cosmos.Uint {
	return cosmos.ZeroUint()
}
