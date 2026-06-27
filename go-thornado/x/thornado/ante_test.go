package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestFinalBTCSourceInputReplayMatchesExistingFinalVariant(t *testing.T) {
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	from := GetRandomBTCAddress()
	to := GetRandomBTCAddress()
	input := common.TxInput{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 100_000_000}
	tx := common.NewTx(
		txID,
		from,
		to,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_990_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
	)
	tx.SourceInputs = []common.TxInput{input}
	observed := common.NewObservedTx(tx, 100, pubKey, 100)
	observed.Sign(signer)

	voter := ObservedTxVoter{
		TxID: txID,
		Txs:  common.ObservedTxs{observed},
	}

	if !isFinalBTCSourceInputReplay(voter, observed, signer) {
		t.Fatal("expected matching final BTC source-input replay to be allowed")
	}
}

func TestFinalBTCSourceInputReplayRejectsDifferentSourceInputs(t *testing.T) {
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	from := GetRandomBTCAddress()
	to := GetRandomBTCAddress()
	tx := common.NewTx(
		txID,
		from,
		to,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(99_990_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
	)
	tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 100_000_000}}
	observed := common.NewObservedTx(tx, 100, pubKey, 100)
	observed.Sign(signer)

	replay := observed
	replay.Tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 100_000_000}}

	voter := ObservedTxVoter{
		TxID: txID,
		Txs:  common.ObservedTxs{observed},
	}

	if isFinalBTCSourceInputReplay(voter, replay, signer) {
		t.Fatal("expected different source-input replay to be rejected")
	}
}
