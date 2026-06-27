package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestTxStagesMempoolPreObservationDoesNotEstimateFromZeroHeight(t *testing.T) {
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(12_000_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(2_320))},
	)
	observed := common.NewObservedTx(tx, 0, GetRandomPubKey(), 1685)

	result := newTxStagesResponse(cosmos.Context{}.WithBlockHeight(4739), ObservedTxVoter{
		TxID:   txID,
		Tx:     observed,
		Txs:    common.ObservedTxs{observed},
		Height: 4737,
	})

	if result.InboundConfirmationCounted == nil {
		t.Fatal("expected confirmation-counting stage")
	}
	if result.InboundConfirmationCounted.Completed {
		t.Fatal("expected zero-height pre-observation to remain pending")
	}
	if got := result.InboundConfirmationCounted.RemainingConfirmationSeconds; got != 0 {
		t.Fatalf("remaining confirmation seconds = %d, want 0", got)
	}
}

