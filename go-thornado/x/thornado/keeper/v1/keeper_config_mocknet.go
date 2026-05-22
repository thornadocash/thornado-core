//go:build mocknet
// +build mocknet

package keeperv1

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

// GetConfig get a config value from key value store
func (k KVStore) GetConfig(ctx cosmos.Context, key string) (int64, error) {
	record := int64(-1)
	_, err := k.getInt64(ctx, k.GetKey(prefixConfig, key), &record)

	// mocknet only fallback to environment variable if unset
	envKey := constants.CamelToSnakeUpper(key)
	envKey = strings.ReplaceAll(envKey, "-", "_") // also handle config with "-" in key
	if record == -1 && os.Getenv(envKey) != "" {
		envValue, err := strconv.ParseInt(os.Getenv(envKey), 10, 64)
		if err != nil {
			return record, fmt.Errorf("invalid config value for %s: %w", envKey, err)
		}
		return envValue, nil
	}

	return record, err
}
