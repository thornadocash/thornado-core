package keeperv1

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

// GetConfigWithRef is a helper function to more readably insert references (such as Asset ConfigString or Chain) into Config key templates.
func (k KVStore) GetConfigWithRef(ctx cosmos.Context, template string, ref ...any) (int64, error) {
	// 'template' should be something like "Halt%sChain" to halt an arbitrary specified chain.
	key := fmt.Sprintf(template, ref...)
	return k.GetConfig(ctx, key)
}

// SetConfig save a config value to key value store
func (k KVStore) SetConfig(ctx cosmos.Context, key string, value int64) {
	k.setInt64(ctx, k.GetKey(prefixConfig, key), value)
}

// GetNodeConfigs get node configs value from key value store
func (k KVStore) GetNodeConfigs(ctx cosmos.Context, key string) (NodeConfigs, error) {
	key = strings.ToUpper(key)
	record := NodeConfigs{}
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(k.GetKey(prefixNodeConfig, key)) {
		return record, nil
	}
	bz := store.Get(k.GetKey(prefixNodeConfig, key))
	if err := k.cdc.Unmarshal(bz, &record); err != nil {
		return NodeConfigs{}, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return record, nil
}

// SetNodeConfig save a config value to key value store for a specific node
func (k KVStore) SetNodeConfig(ctx cosmos.Context, key string, value int64, acc cosmos.AccAddress) error {
	key = strings.ToUpper(key)
	kvkey := k.GetKey(prefixNodeConfig, key)
	record, err := k.GetNodeConfigs(ctx, key)
	if err != nil {
		return err
	}
	record.Set(key, value, acc)

	// delete the node config if value is negative
	if value < 0 {
		record.Delete(key, acc)
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil || len(record.Configs) == 0 {
		store.Delete(kvkey)
	} else {
		store.Set(kvkey, buf)
	}
	return err
}

// DeleteNodeConfigs deletes all node config votes for a given key
func (k KVStore) DeleteNodeConfigs(ctx cosmos.Context, key string) {
	k.del(ctx, k.GetKey(prefixNodeConfig, key))
}

func (k KVStore) PurgeOperationalNodeConfigs(ctx cosmos.Context) {
	iterNode := k.GetNodeConfigIterator(ctx)
	defer iterNode.Close()
	for ; iterNode.Valid(); iterNode.Next() {
		key := strings.TrimPrefix(string(iterNode.Key()), string(prefixNodeConfig)+"/")
		if k.IsOperationalConfig(key) {
			k.DeleteNodeConfigs(ctx, key)
		}
	}
}

// GetConfigIterator iterate gas units
func (k KVStore) GetConfigIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixConfig)
}

// GetNodeConfigIterator iterate gas units
func (k KVStore) GetNodeConfigIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixNodeConfig)
}

func (k KVStore) DeleteConfig(ctx cosmos.Context, key string) error {
	k.del(ctx, k.GetKey(prefixConfig, key))
	return nil
}

func (k KVStore) GetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) int64 {
	record := int64(-1)
	_, _ = k.getInt64(ctx, k.GetKey(prefixNodePauseChain, acc.String()), &record)
	return record
}

func (k KVStore) SetNodePauseChain(ctx cosmos.Context, acc cosmos.AccAddress) {
	k.setInt64(ctx, k.GetKey(prefixNodePauseChain, acc.String()), ctx.BlockHeight())
}

func (k KVStore) IsOperationalConfig(key string) bool {
	exactMatches := []string{
		constants.ConfigKeyEnableFrostBTC,
	}
	for i := range exactMatches {
		if strings.EqualFold(key, exactMatches[i]) {
			return true
		}
	}

	exactUnmatches := []string{
		constants.Chain_PauseNodeBlocks.String(),
		constants.Slash_PauseThreshold.String(),
	}
	for i := range exactUnmatches {
		if strings.EqualFold(key, exactUnmatches[i]) {
			return false
		}
	}

	// Past this point, compare only upper-case strings due to case sensitivity.
	key = strings.ToUpper(key)
	partialMatches := []string{
		"HALT",
		"PAUSE",
		"STOPSOLVENCYCHECK",
	}
	for i := range partialMatches {
		if strings.Contains(key, partialMatches[i]) {
			return true
		}
	}

	return false
}
