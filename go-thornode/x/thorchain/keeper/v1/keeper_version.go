package keeperv1

import (
	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/runtime"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// GetVersionWithCtx returns the version with the given context,
// and returns true if the version was found in the store
func (k KVStore) GetVersionWithCtx(ctx cosmos.Context) (semver.Version, bool) {
	key := k.GetKey(prefixVersion, "")
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	val := store.Get(key)
	if val == nil {
		return semver.Version{}, false
	}
	return semver.MustParse(string(val)), true
}

// SetVersionWithCtx stores the version
func (k KVStore) SetVersionWithCtx(ctx cosmos.Context, v semver.Version) {
	key := k.GetKey(prefixVersion, "")
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(key, []byte(v.String()))
}

// getMinJoinLast returns the last stored MinJoinVersion
func (k KVStore) getMinJoinLast(ctx cosmos.Context) MinJoinLast {
	key := k.GetKey(prefixMinJoinLast, "")
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	minJoinLastBytes := store.Get(key)
	record := MinJoinLast{}
	if err := k.cdc.Unmarshal(minJoinLastBytes, &record); err != nil {
		ctx.Logger().Error("failed to unmarshal MinJoinLast from KVStore", "error", err, "key", key)
		return MinJoinLast{}
	}
	return record
}

// setMinJoinLast stores the MinJoinVersion
func (k KVStore) setMinJoinLast(ctx cosmos.Context, record MinJoinLast) {
	key := k.GetKey(prefixMinJoinLast, "")
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

// GetMinJoinLast returns the last stored MinJoinVersion and its last stored height,
// or else (at worse performance) the current MinJoinVersion and 0.
func (k KVStore) GetMinJoinLast(ctx cosmos.Context) (semver.Version, int64) {
	minJoinLast := k.getMinJoinLast(ctx)

	version := semver.Version{}
	if minJoinLast.Version != "" {
		version = semver.MustParse(minJoinLast.Version)
	}

	lastHeight := minJoinLast.LastChangedHeight

	// MinJoinLast is intended to be kept updated to always equal the MinJoinVersion.
	// If either the stored version or height indicates that unstored/unset, fall back to GetMinJoinVersion.
	if version.Equals(semver.Version{}) || lastHeight <= 0 {
		return k.GetMinJoinVersion(ctx), 0
	}

	return version, lastHeight
}

// SetMinJoinLast updates-if-changed the MinJoinVersion and its height when changed.
func (k KVStore) SetMinJoinLast(ctx cosmos.Context) {
	minJoinVersion := k.GetMinJoinVersion(ctx)
	minJoinLast, lastHeight := k.GetMinJoinLast(ctx)

	// GetMinJoinLast will fall back to GetMinJoinVersion if unset (returning a height of 0),
	// so check both the version and the last changed height.
	if !minJoinVersion.Equals(minJoinLast) || lastHeight <= 0 {
		// Since different (or unset), update it.
		record := MinJoinLast{LastChangedHeight: ctx.BlockHeight(), Version: minJoinVersion.String()}
		k.setMinJoinLast(ctx, record)
	}
}
