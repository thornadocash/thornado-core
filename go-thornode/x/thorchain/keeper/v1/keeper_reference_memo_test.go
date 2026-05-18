package keeperv1

import (
	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

type KeeperReferenceMemoSuite struct{}

var _ = Suite(&KeeperReferenceMemoSuite{})

func (s *KeeperReferenceMemoSuite) TestReferenceMemo(c *C) {
	ctx, k := setupKeeperForTest(c)
	var err error
	ref := "123456"
	asset := common.BTCAsset
	signer := GetRandomBech32Addr()
	hash := GetRandomTxHash()

	ok := k.ReferenceMemoExists(ctx, asset, ref)
	c.Assert(ok, Equals, false)

	refMemo := NewReferenceMemo(asset, "my memo", ref, 35)
	refMemo.RegistrationHash = hash
	refMemo.RegisteredBy = signer
	k.SetReferenceMemo(ctx, refMemo)

	ok = k.ReferenceMemoExists(ctx, asset, ref)
	c.Assert(ok, Equals, true)
	ok = k.ReferenceMemoExists(ctx, asset, "123456789")
	c.Assert(ok, Equals, false)
	ok = k.ReferenceMemoExists(ctx, common.ETHAsset, ref)
	c.Assert(ok, Equals, false)

	refMemo, err = k.GetReferenceMemo(ctx, asset, ref)
	c.Assert(err, IsNil)
	c.Check(refMemo.Asset.Equals(common.BTCAsset), Equals, true)
	c.Check(refMemo.Memo, Equals, "my memo")
	c.Check(refMemo.Reference, Equals, ref)
	c.Check(refMemo.Height, Equals, int64(35))
	c.Check(refMemo.RegisteredBy.String(), Equals, signer.String())
	c.Check(refMemo.RegistrationHash.String(), Equals, hash.String())

	refMemo, err = k.GetReferenceMemoByTxnHash(ctx, hash)
	c.Assert(err, IsNil)
	c.Check(refMemo.Asset.Equals(common.BTCAsset), Equals, true)
	c.Check(refMemo.Memo, Equals, "my memo")
	c.Check(refMemo.Reference, Equals, ref)
	c.Check(refMemo.Height, Equals, int64(35))
	c.Check(refMemo.RegisteredBy.String(), Equals, signer.String())
	c.Check(refMemo.RegistrationHash.String(), Equals, hash.String())

	// Lookup by non-existent hash should return an error
	unknownHash := GetRandomTxHash()
	_, err = k.GetReferenceMemoByTxnHash(ctx, unknownHash)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, "reference memo not found for hash:.*")
}

func (s *KeeperReferenceMemoSuite) TestSetHashAliasEdgeCases(c *C) {
	ctx, k := setupKeeperForTest(c)
	asset := common.BTCAsset
	ref := "test123"

	// Test case 1: Empty hash should return early without setting anything
	emptyHash := common.TxID("")
	originalRefMemo := NewReferenceMemo(asset, "original memo", ref, 100)

	k.setHashAlias(ctx, emptyHash, originalRefMemo.Key())

	// Verify that nothing was set for empty hash
	retrievedKey := k.getHashAlias(ctx, emptyHash)
	c.Check(retrievedKey, Equals, "")

	// Test case 2: Existing hash should not be overwritten
	existingHash := GetRandomTxHash()
	firstRefMemo := NewReferenceMemo(asset, "first memo", "first_ref", 50)
	firstRefMemo.RegistrationHash = existingHash

	// Set the first memo with the hash
	k.setHashAlias(ctx, existingHash, firstRefMemo.Key())

	// Verify the first memo was set
	retrievedKey = k.getHashAlias(ctx, existingHash)
	c.Check(retrievedKey, Equals, firstRefMemo.Key())

	// Try to overwrite with a different memo (should fail)
	secondRefMemo := NewReferenceMemo(asset, "second memo", "second_ref", 75)
	secondRefMemo.RegistrationHash = existingHash

	k.setHashAlias(ctx, existingHash, secondRefMemo.Key())

	// Verify the original memo is still there (not overwritten)
	retrievedKey = k.getHashAlias(ctx, existingHash)
	c.Check(retrievedKey, Equals, firstRefMemo.Key())
	c.Check(retrievedKey, Not(Equals), secondRefMemo.Key())
}
