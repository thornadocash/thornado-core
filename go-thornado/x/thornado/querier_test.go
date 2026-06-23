package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestSelectQueryObservedTxVoterPrefersFinalOutbound(t *testing.T) {
	txID := common.TxID("32D369E844293F90DBA733BE55B28A784B8646D8380A3938B0AA55B67A315254")
	tx := common.NewTx(
		txID,
		common.Address("bcrt1pfrom"),
		common.Address("bcrt1pto"),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
	)
	pubKey := common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz")

	inbound := types.NewObservedTxVoter(txID, common.ObservedTxs{
		common.NewObservedTx(tx, 0, pubKey, 10),
	})
	outbound := types.NewObservedTxVoter(txID, common.ObservedTxs{
		common.NewObservedTx(tx, 10, pubKey, 10),
	})
	outbound.FinalisedHeight = 12
	outbound.Tx = outbound.Txs[0]

	got := selectQueryObservedTxVoter(inbound, outbound)
	if got.FinalisedHeight != outbound.FinalisedHeight {
		t.Fatalf("expected finalized outbound voter, got finalised height %d", got.FinalisedHeight)
	}
}
