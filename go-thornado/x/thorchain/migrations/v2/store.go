package v2

import (
	"fmt"
	"strings"

	"cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	prefixStoreVersion = "_ver/"
)

// MigrateStore performs in-place store migrations from v2.137.3 to v3.0.0
// migration includes:
//
// - Remove legacy store migration version
func MigrateStore(ctx sdk.Context, storeService store.KVStoreService) error {
	store := storeService.OpenKVStore(ctx)
	ok, err := store.Has(getKey(prefixStoreVersion, ""))
	if err != nil {
		return err
	}
	if ok {
		return store.Delete(getKey(prefixStoreVersion, ""))
	}

	return nil
}

func getKey(prefix, key string) []byte {
	return []byte(fmt.Sprintf("%s/%s", prefix, strings.ToUpper(key)))
}
