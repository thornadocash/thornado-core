//go:build !mocknet
// +build !mocknet

package keeperv1

import "gitlab.com/thorchain/thornode/v3/common/cosmos"

// GetMimir get a mimir value from key value store
func (k KVStore) GetMimir(ctx cosmos.Context, key string) (int64, error) {
	record := int64(-1)
	_, err := k.getInt64(ctx, k.GetKey(prefixMimir, key), &record)
	return record, err
}
