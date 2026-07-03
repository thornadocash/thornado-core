package thornado

import (
	"testing"

	"cosmossdk.io/log"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// storeMigrateKeeperFake implements storeMigrateKeeper for dispatch tests.
type storeMigrateKeeperFake struct {
	configs map[string]int64
	vaults  map[string]Vault
	txouts  map[int64]*TxOut
}

func newStoreMigrateKeeperFake() *storeMigrateKeeperFake {
	return &storeMigrateKeeperFake{
		configs: map[string]int64{},
		vaults:  map[string]Vault{},
		txouts:  map[int64]*TxOut{},
	}
}

func (k *storeMigrateKeeperFake) SetConfig(_ cosmos.Context, key string, value int64) {
	k.configs[key] = value
}

func (k *storeMigrateKeeperFake) GetVault(_ cosmos.Context, pk common.PubKey) (Vault, error) {
	return k.vaults[pk.String()], nil
}

func (k *storeMigrateKeeperFake) SetVault(_ cosmos.Context, v Vault) error {
	k.vaults[v.PubKey.String()] = v
	return nil
}

func (k *storeMigrateKeeperFake) GetTxOut(_ cosmos.Context, height int64) (*TxOut, error) {
	if t, ok := k.txouts[height]; ok {
		return t, nil
	}
	return &TxOut{Height: height}, nil
}

func (k *storeMigrateKeeperFake) SetTxOut(_ cosmos.Context, t *TxOut) error {
	k.txouts[t.Height] = t
	return nil
}

func TestParseStoreMigrateTarget(t *testing.T) {
	ok := []string{
		"CONFIG:HALTSIGNINGBTC",
		"config:HALTSIGNINGBTC",
		"VAULTCOIN:tthorpub1xyz:BTC.BTC",
		"VAULTSTATUS:tthorpub1xyz",
		"TXOUTCANCEL:63391:0",
	}
	for _, k := range ok {
		if _, err := parseStoreMigrateTarget(k); err != nil {
			t.Fatalf("expected %q to parse, got %v", k, err)
		}
	}
	bad := []string{"", "FOO:bar", "CONFIG", "VAULTCOIN:onlyone", "TXOUTCANCEL:1"}
	for _, k := range bad {
		if _, err := parseStoreMigrateTarget(k); err == nil {
			t.Fatalf("expected %q to be rejected", k)
		}
	}
}

func TestApplyStoreMigrationConfig(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := newStoreMigrateKeeperFake()
	if err := applyStoreMigration(ctx, k, "CONFIG:HALTSIGNINGBTC", "0"); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	if k.configs["HALTSIGNINGBTC"] != 0 {
		t.Fatalf("expected HALTSIGNINGBTC=0, got %d", k.configs["HALTSIGNINGBTC"])
	}
	if err := applyStoreMigration(ctx, k, "CONFIG:HALTSIGNINGBTC", "notint"); err == nil {
		t.Fatalf("expected non-int config value to error")
	}
}

func TestApplyStoreMigrationVaultCoinSetsExactAmount(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	pk := GetRandomPubKey()
	k := newStoreMigrateKeeperFake()
	k.vaults[pk.String()] = Vault{
		PubKey: pk,
		Status: RetiringVault,
		Coins:  common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(12_300_000_000))),
	}
	// Correct phantom balance down to the true wallet amount.
	if err := applyStoreMigration(ctx, k, "VAULTCOIN:"+pk.String()+":BTC.BTC", "70000000"); err != nil {
		t.Fatalf("apply vaultcoin: %v", err)
	}
	got := k.vaults[pk.String()].GetCoin(common.BTCAsset).Amount
	if !got.Equal(cosmos.NewUint(70000000)) {
		t.Fatalf("expected 70000000, got %s", got)
	}
	// Draining to zero flips a retiring vault inactive.
	if err := applyStoreMigration(ctx, k, "VAULTCOIN:"+pk.String()+":BTC.BTC", "0"); err != nil {
		t.Fatalf("apply vaultcoin zero: %v", err)
	}
	if k.vaults[pk.String()].Status != InactiveVault {
		t.Fatalf("expected drained retiring vault to be inactive, got %v", k.vaults[pk.String()].Status)
	}
}

func TestApplyStoreMigrationTxOutCancelRemovesItem(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(64000).WithLogger(log.NewNopLogger())
	k := newStoreMigrateKeeperFake()
	k.txouts[63391] = &TxOut{
		Height: 63391,
		TxArray: []TxOutItem{
			{Chain: common.BTCChain, TxType: "refund", Coin: common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
			{Chain: common.BTCChain, TxType: "out", Coin: common.NewCoin(common.BTCAsset, cosmos.NewUint(2))},
		},
	}
	if err := applyStoreMigration(ctx, k, "TXOUTCANCEL:63391:0", "1"); err != nil {
		t.Fatalf("apply txoutcancel: %v", err)
	}
	remaining := k.txouts[63391].TxArray
	if len(remaining) != 1 || remaining[0].TxType != "out" {
		t.Fatalf("expected only the 'out' item to remain, got %+v", remaining)
	}
	// Out-of-range index is rejected.
	if err := applyStoreMigration(ctx, k, "TXOUTCANCEL:63391:9", "1"); err == nil {
		t.Fatalf("expected out-of-range index to error")
	}
}
