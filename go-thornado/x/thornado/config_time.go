package thornado

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

type configInt64Getter interface {
	GetConfigInt64(cosmos.Context, constants.ConfigName) int64
}

func getConfigDurationBlocks(ctx cosmos.Context, k configInt64Getter, key constants.ConfigName) int64 {
	return constants.MinutesToBlocks(k.GetConfigInt64(ctx, key), k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds))
}
