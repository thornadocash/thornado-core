//go:build !mocknet
// +build !mocknet

package keeperv1

import "github.com/thornadocash/go-thornado/common/cosmos"

// GetConfig get a config value from key value store
func (k KVStore) GetConfig(ctx cosmos.Context, key string) (int64, error) {
	record := int64(-1)
	_, err := k.getInt64(ctx, k.GetKey(prefixConfig, key), &record)
	return record, err
}
