package xrp

import (
	"sync"

	"gitlab.com/thorchain/thornode/v3/common"
)

type XrpMetadata struct {
	SeqNumber   int64
	BlockHeight int64
}

type XrpMetaDataStore struct {
	lock  *sync.Mutex
	accts map[common.PubKey]XrpMetadata
}

func NewXrpMetaDataStore() *XrpMetaDataStore {
	return &XrpMetaDataStore{
		lock:  &sync.Mutex{},
		accts: make(map[common.PubKey]XrpMetadata),
	}
}

func (b *XrpMetaDataStore) Get(pk common.PubKey) XrpMetadata {
	b.lock.Lock()
	defer b.lock.Unlock()
	if val, ok := b.accts[pk]; ok {
		return val
	}
	return XrpMetadata{}
}

func (b *XrpMetaDataStore) Set(pk common.PubKey, meta XrpMetadata) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.accts[pk] = meta
}

func (b *XrpMetaDataStore) SeqInc(pk common.PubKey) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if meta, ok := b.accts[pk]; ok {
		meta.SeqNumber++
		b.accts[pk] = meta
	}
}
