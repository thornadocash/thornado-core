package thornado

import (
	"fmt"
	"testing"

	"github.com/cometbft/cometbft/crypto/secp256k1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func BenchmarkBTCSelectVaultSourceInputs(b *testing.B) {
	const candidateCount = 512

	ctx := testContext(candidateCount)
	k := newShielderFlowTestKeeper()
	k.configs[constants.UTXO_MaxSpendCount] = 128

	vaultPubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		b.Fatal(err)
	}
	sourceAddr, err := common.NewAddress("bcrt1pfrs56cns3k4nvt7wkym80kddmctklp2ajcce8vept6wyqt8p4n9syx5h94")
	if err != nil {
		b.Fatal(err)
	}
	vault := Vault{PubKey: vaultPubKey}

	for i := 0; i < candidateCount; i++ {
		txID, err := common.NewTxID(fmt.Sprintf("%064x", i+1))
		if err != nil {
			b.Fatal(err)
		}
		item := TxOutItem{
			Chain:       common.BTCChain,
			VaultPubKey: vaultPubKey,
			ToAddress:   sourceAddr,
			Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(uint64(10_000_000+i))),
			OutHash:     txID,
			OutVout:     uint32(i % 3),
		}
		if i%11 == 0 && i > 0 {
			spentTxID, err := common.NewTxID(fmt.Sprintf("%064x", i))
			if err != nil {
				b.Fatal(err)
			}
			item.SourceInputs = []types.TxOutInput{{
				TxId:       spentTxID,
				Vout:       uint32((i - 1) % 3),
				AmountSats: uint64(10_000_000 + i - 1),
			}}
		}
		if err := k.SetTxOut(ctx, &TxOut{
			Height:  int64(i + 1),
			TxArray: []TxOutItem{item},
		}); err != nil {
			b.Fatal(err)
		}
	}

	inputs := btcSelectVaultSourceInputs(ctx, k, vault, sourceAddr, cosmos.NewUint(1_000_000_000), 0)
	if len(inputs) == 0 {
		b.Fatal("expected source inputs")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inputs = btcSelectVaultSourceInputs(ctx, k, vault, sourceAddr, cosmos.NewUint(1_000_000_000), 0)
		if len(inputs) == 0 {
			b.Fatal("expected source inputs")
		}
	}
}

func benchmarkTxOutInputs(size int) ([]types.TxOutInput, []types.TxOutInput) {
	a := make([]types.TxOutInput, size)
	b := make([]types.TxOutInput, size)
	for i := range a {
		txID, err := common.NewTxID(fmt.Sprintf("%064x", i+1))
		if err != nil {
			panic(err)
		}
		input := types.TxOutInput{
			TxId:       txID,
			Vout:       uint32(i % 4),
			AmountSats: uint64(10_000_000 + i),
		}
		a[i] = input
		b[size-1-i] = input
	}
	return a, b
}

func BenchmarkBTCTxOutInputsEqual(b *testing.B) {
	a, reversed := benchmarkTxOutInputs(128)
	if !btcTxOutInputsEqual(a, reversed) {
		b.Fatal("expected source input sets to match")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !btcTxOutInputsEqual(a, reversed) {
			b.Fatal("expected source input sets to match")
		}
	}
}

func BenchmarkBTCSourceInputsAmount(b *testing.B) {
	inputs, _ := benchmarkTxOutInputs(128)
	expected := cosmos.ZeroUint()
	for _, input := range inputs {
		expected = expected.Add(cosmos.NewUint(input.AmountSats))
	}
	if got := btcSourceInputsAmount(inputs); !got.Equal(expected) {
		b.Fatalf("expected %s, got %s", expected, got)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := btcSourceInputsAmount(inputs); !got.Equal(expected) {
			b.Fatalf("expected %s, got %s", expected, got)
		}
	}
}
