//go:build mocknet
// +build mocknet

package keeperv1

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

// GetMimir get a mimir value from key value store
func (k KVStore) GetMimir(ctx cosmos.Context, key string) (int64, error) {
	record := int64(-1)
	_, err := k.getInt64(ctx, k.GetKey(prefixMimir, key), &record)

	// mocknet only fallback to environment variable if unset
	envKey := constants.CamelToSnakeUpper(key)
	envKey = strings.ReplaceAll(envKey, "-", "_") // also handle mimir with "-" in key
	if record == -1 && os.Getenv(envKey) != "" {
		envValue, err := strconv.ParseInt(os.Getenv(envKey), 10, 64)
		if err != nil {
			return record, fmt.Errorf("invalid mimir value for %s: %w", envKey, err)
		}
		return envValue, nil
	}

	return record, err
}
