package types

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func NewSecuredAsset(asset common.Asset) SecuredAsset {
	return SecuredAsset{
		Asset: asset,
		Depth: cosmos.ZeroUint(),
	}
}

func (tu SecuredAsset) Key() string {
	return tu.Asset.String()
}
