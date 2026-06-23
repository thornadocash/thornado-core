package signer

import (
	"testing"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type SignerSuite struct{}

var _ = Suite(&SignerSuite{})

func (s *SignerSuite) TestTxOutCompletionMatchIgnoresMutableSigningFields(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199982710))),
		InHash:           common.BlankTxID,
		GasRate:          13,
		TxType:           "migrate",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 99_997_855},
		},
	}
	completed := item
	completed.SourceInputs = append([]types.TxOutInput(nil), item.SourceInputs...)
	completed.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199992681)))
	completed.GasRate = 19

	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].AmountSats++
	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].Vout++
	c.Assert(txOutCompletionMatch(item, completed), Equals, true)
}

func (s *SignerSuite) TestTxOutCompletionMatchKeepsExternalTxoutsStrict(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))),
		InHash:           ttypes.GetRandomTxHash(),
		TxType:           "outbound",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 100_000},
		},
	}
	completed := item
	completed.SourceInputs = append([]types.TxOutInput(nil), item.SourceInputs...)

	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].AmountSats++
	c.Assert(txOutCompletionMatch(item, completed), Equals, false)
}

func (s *SignerSuite) TestCompletedTxOutItemUsesHeightScopedHistory(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	outHash := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199982710))),
		InHash:           common.BlankTxID,
		GasRate:          13,
		TxType:           "migrate",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 99_997_855},
		},
	}
	completed := types.TxArrayItem{
		Chain:        item.Chain,
		ToAddress:    item.ToAddress,
		VaultPubKey:  item.VaultPubKey,
		Coin:         item.Coins[0],
		GasRate:      item.GasRate,
		InHash:       item.InHash,
		OutHash:      outHash,
		TxType:       item.TxType,
		SourceInputs: item.SourceInputs,
	}

	matched, matchedOutHash := completedTxOutItem(NewTxOutStoreItem(127, item, 0), []types.TxOut{
		{Height: 126, TxArray: []types.TxArrayItem{completed}},
	})
	c.Assert(matched, Equals, false)
	c.Assert(matchedOutHash.IsEmpty(), Equals, true)

	matched, matchedOutHash = completedTxOutItem(NewTxOutStoreItem(127, item, 0), []types.TxOut{
		{Height: 127, TxArray: []types.TxArrayItem{completed}},
	})
	c.Assert(matched, Equals, true)
	c.Assert(matchedOutHash.Equals(outHash), Equals, true)
}

func (s *SignerSuite) TestTxOutIsStaleKeepsLegacyRescheduleBuffer(c *C) {
	c.Assert(txOutIsStale(831, 675, 300, 144, false), Equals, false)
	c.Assert(txOutIsStale(832, 675, 300, 144, false), Equals, true)
}

func (s *SignerSuite) TestTxOutIsStaleAllowsFrostSignerRotation(c *C) {
	c.Assert(txOutIsStale(831, 675, 300, 144, true), Equals, false)
	c.Assert(txOutIsStale(1119, 675, 300, 144, true), Equals, false)
	c.Assert(txOutIsStale(1120, 675, 300, 144, true), Equals, true)
}

func (s *SignerSuite) TestTxOutIsStaleIgnoresInvalidSigningPeriod(c *C) {
	c.Assert(txOutIsStale(831, 675, 0, 144, false), Equals, false)
	c.Assert(txOutIsStale(831, 675, 0, 144, true), Equals, false)
}
