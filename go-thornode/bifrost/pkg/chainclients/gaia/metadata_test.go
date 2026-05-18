package gaia

import (
	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

type MetadataTestSuite struct{}

var _ = Suite(&MetadataTestSuite{})

func (s *MetadataTestSuite) TestNewCosmosMetaDataStore(c *C) {
	store := NewCosmosMetaDataStore()
	c.Assert(store, NotNil)
	c.Assert(store.accts, NotNil)
}

func (s *MetadataTestSuite) TestGetSetMetadata(c *C) {
	store := NewCosmosMetaDataStore()

	pk := common.PubKey("thorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzy9zsp6nhe2lgmf7ue08nxhm5frcg76v")

	// get from empty store returns zero value
	meta := store.Get(pk)
	c.Check(meta.AccountNumber, Equals, int64(0))
	c.Check(meta.SeqNumber, Equals, int64(0))
	c.Check(meta.BlockHeight, Equals, int64(0))

	// set and get
	store.Set(pk, CosmosMetadata{
		AccountNumber: 42,
		SeqNumber:     10,
		BlockHeight:   100,
	})
	meta = store.Get(pk)
	c.Check(meta.AccountNumber, Equals, int64(42))
	c.Check(meta.SeqNumber, Equals, int64(10))
	c.Check(meta.BlockHeight, Equals, int64(100))

	// overwrite
	store.Set(pk, CosmosMetadata{
		AccountNumber: 42,
		SeqNumber:     11,
		BlockHeight:   101,
	})
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(11))
}

func (s *MetadataTestSuite) TestGetByAccount(c *C) {
	store := NewCosmosMetaDataStore()

	pk := common.PubKey("thorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzy9zsp6nhe2lgmf7ue08nxhm5frcg76v")

	// not found returns zero value
	meta := store.GetByAccount(42)
	c.Check(meta.AccountNumber, Equals, int64(0))

	store.Set(pk, CosmosMetadata{
		AccountNumber: 42,
		SeqNumber:     5,
		BlockHeight:   50,
	})

	meta = store.GetByAccount(42)
	c.Check(meta.AccountNumber, Equals, int64(42))
	c.Check(meta.SeqNumber, Equals, int64(5))

	// non-existent account
	meta = store.GetByAccount(999)
	c.Check(meta.AccountNumber, Equals, int64(0))
}

func (s *MetadataTestSuite) TestSeqInc(c *C) {
	store := NewCosmosMetaDataStore()

	pk := common.PubKey("thorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzy9zsp6nhe2lgmf7ue08nxhm5frcg76v")

	// inc on non-existent key does nothing
	store.SeqInc(pk)
	meta := store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(0))

	store.Set(pk, CosmosMetadata{
		AccountNumber: 1,
		SeqNumber:     10,
		BlockHeight:   100,
	})

	store.SeqInc(pk)
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(11))

	store.SeqInc(pk)
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(12))
}
