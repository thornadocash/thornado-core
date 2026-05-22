package keeperv1

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

// GetConstants returns the constant values
func (k KVStore) GetConstants() constants.ConfigValues {
	return k.constAccessor
}

// GetConfigInt64 returns the config value for the key if set, otherwise the constant value
func (k KVStore) GetConfigInt64(ctx cosmos.Context, key constants.ConfigName) int64 {
	val, err := k.GetConfig(ctx, key.String())
	if val < 0 || err != nil {
		val = k.GetConstants().GetInt64Value(key)
		if err != nil {
			ctx.Logger().Error("fail to get config", "key", key.String(), "error", err)
		}
	}
	return val
}
