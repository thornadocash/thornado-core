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

func TestFinalBTCObservedTxReplayMatchesWithoutSourceInputs(t *testing.T) {
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(5_000))},
	)
	observed := common.NewObservedTx(tx, 100, pubKey, 100)
	observed.Sign(signer)

	voter := ObservedTxVoter{
		TxID: txID,
		Tx:   observed,
		Txs:  common.ObservedTxs{observed},
	}

	if !isFinalBTCObservedTxReplay(voter, observed, signer) {
		t.Fatal("expected matching final BTC replay without source inputs to be allowed")
	}
}

func TestReserveObservedTxAttestationsAllowsInboundFinalBTCReplay(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(5_000))},
	)
	tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 20_005_000}}
	observed := common.NewObservedTx(tx, 100, pubKey, 100)
	observed.Sign(signer)
	voter := ObservedTxVoter{
		TxID:            txID,
		Tx:              observed,
		Txs:             common.ObservedTxs{observed},
		FinalisedHeight: 100,
	}

	if err := reserveObservedTxAttestations(ctx, k, voter, observed, []cosmos.AccAddress{signer}, true); err != nil {
		t.Fatalf("expected inbound final BTC replay through ante: %v", err)
	}
}

func TestReserveObservedTxAttestationsAllowsExactBTCReplayBeforeFinal(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(5_000))},
	)
	tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 20_005_000}}
	observed := common.NewObservedTx(tx, 100, pubKey, 110)
	observed.Sign(signer)
	voter := ObservedTxVoter{
		TxID: txID,
		Txs:  common.ObservedTxs{observed},
	}

	if err := reserveObservedTxAttestations(ctx, k, voter, observed, []cosmos.AccAddress{signer}, true); err != nil {
		t.Fatalf("expected exact BTC replay before finalisation through ante: %v", err)
	}
}

func TestReserveObservedTxAttestationsRejectsBTCReplayWithChangedSourceInputs(t *testing.T) {
	ctx := testContext(100)
	k := newShielderFlowTestKeeper()
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		GetRandomBTCAddress(),
		GetRandomBTCAddress(),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(5_000))},
	)
	tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 20_005_000}}
	observed := common.NewObservedTx(tx, 100, pubKey, 110)
	observed.Sign(signer)

	replay := observed
	replay.Tx.SourceInputs = []common.TxInput{{TxID: GetRandomTxHash(), Vout: 0, AmountSats: 20_005_000}}
	voter := ObservedTxVoter{
		TxID: txID,
		Txs:  common.ObservedTxs{observed},
	}

	if err := reserveObservedTxAttestations(ctx, k, voter, replay, []cosmos.AccAddress{signer}, true); err == nil {
		t.Fatal("expected BTC replay with changed source inputs to be rejected")
	}
}
