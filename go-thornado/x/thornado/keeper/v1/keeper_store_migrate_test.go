package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

type KeeperStoreMigrateSuite struct{}

var _ = Suite(&KeeperStoreMigrateSuite{})

// The core safety property: a raw KVSET can never leave bytes under a store
// prefix that the reader for that prefix cannot decode (which would panic every
// node at the same block).
func (s *KeeperStoreMigrateSuite) TestValidateRawStoreValueClosesPanicVector(c *C) {
	_, k := setupKeeperForTest(c)

	vaultKey := k.GetKey(prefixVault, "somepubkey")

	// Garbage bytes under the vault prefix are refused.
	c.Assert(k.ValidateRawStoreValue(vaultKey, []byte{0xff, 0x00, 0x99, 0x01}), NotNil)

	// A real, marshaled Vault under the vault prefix is accepted, and once
	// written it round-trips through the normal typed reader (i.e. no panic).
	vault := NewVault(1, ActiveVault, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings())
	vault.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1234)))
	bz := k.cdc.MustMarshal(&vault)
	c.Assert(k.ValidateRawStoreValue(vaultKey, bz), IsNil)
}

func (s *KeeperStoreMigrateSuite) TestSetRawStoreValueWritesValidBytes(c *C) {
	ctx, k := setupKeeperForTest(c)

	pk := GetRandomPubKey()
	vaultKey := k.GetKey(prefixVault, pk.String())
	vault := NewVault(1, ActiveVault, BaseVault, pk, common.Chains{common.BTCChain}.Strings())
	vault.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(5000)))
	bz := k.cdc.MustMarshal(&vault)

	c.Assert(k.SetRawStoreValue(ctx, vaultKey, bz), IsNil)

	got, err := k.GetVault(ctx, pk)
	c.Assert(err, IsNil)
	c.Assert(got.GetCoin(common.BTCAsset).Amount.Equal(cosmos.NewUint(5000)), Equals, true)

	// Undecodable bytes are refused and nothing is written.
	badKey := k.GetKey(prefixVault, "otherpubkey")
	c.Assert(k.SetRawStoreValue(ctx, badKey, []byte{0x01, 0x02, 0x03}), NotNil)
	c.Assert(k.has(ctx, badKey), Equals, false)
}

func (s *KeeperStoreMigrateSuite) TestRawStoreRejectsUnknownPrefix(c *C) {
	ctx, k := setupKeeperForTest(c)
	unknown := []byte("totally_unknown_prefix/x")
	c.Assert(k.ValidateRawStoreValue(unknown, []byte("anything")), NotNil)
	c.Assert(k.SetRawStoreValue(ctx, unknown, []byte("anything")), NotNil)
	c.Assert(k.ValidateRawStoreKey(unknown), NotNil)
}

func (s *KeeperStoreMigrateSuite) TestStoreMigrateVoteRoundTrip(c *C) {
	ctx, k := setupKeeperForTest(c)
	a1 := GetRandomBech32Addr()
	a2 := GetRandomBech32Addr()

	k.SetStoreMigrateVote(ctx, "CONFIG:HALTSIGNINGBTC", "0", a1)
	k.SetStoreMigrateVote(ctx, "CONFIG:HALTSIGNINGBTC", "0", a2)
	votes := k.GetStoreMigrateVotes(ctx, "CONFIG:HALTSIGNINGBTC")
	c.Assert(len(votes.Votes), Equals, 2)
	c.Assert(votes.Votes[a1.String()], Equals, "0")

	// A node can change its vote.
	k.SetStoreMigrateVote(ctx, "CONFIG:HALTSIGNINGBTC", "5", a1)
	votes = k.GetStoreMigrateVotes(ctx, "CONFIG:HALTSIGNINGBTC")
	c.Assert(votes.Votes[a1.String()], Equals, "5")

	_, ok := k.GetStoreMigrateApplied(ctx, "CONFIG:HALTSIGNINGBTC")
	c.Assert(ok, Equals, false)
	k.SetStoreMigrateApplied(ctx, "CONFIG:HALTSIGNINGBTC", "0")
	applied, ok := k.GetStoreMigrateApplied(ctx, "CONFIG:HALTSIGNINGBTC")
	c.Assert(ok, Equals, true)
	c.Assert(applied, Equals, "0")

	k.DeleteStoreMigrateVotes(ctx, "CONFIG:HALTSIGNINGBTC")
	votes = k.GetStoreMigrateVotes(ctx, "CONFIG:HALTSIGNINGBTC")
	c.Assert(len(votes.Votes), Equals, 0)
}
