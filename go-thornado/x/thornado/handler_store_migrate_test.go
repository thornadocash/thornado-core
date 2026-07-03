package thornado

import (
	"encoding/hex"
	"testing"

	"cosmossdk.io/log"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// storeMigrateKeeperFake implements storeMigrateKeeper for dispatch tests. The
// raw KVSET/KVDEL surface mirrors the real allowlist+decode guard: only keys
// under an allowlisted prefix are accepted, and (for the vault prefix) the
// value must be non-empty to stand in for "decodes as a Vault".
type storeMigrateKeeperFake struct {
	configs map[string]int64
	vaults  map[string]Vault
	txouts  map[int64]*TxOut
	raw     map[string][]byte
}

func newStoreMigrateKeeperFake() *storeMigrateKeeperFake {
	return &storeMigrateKeeperFake{
		configs: map[string]int64{},
		vaults:  map[string]Vault{},
		txouts:  map[int64]*TxOut{},
		raw:     map[string][]byte{},
	}
}

func (k *storeMigrateKeeperFake) ValidateRawStoreKey(key []byte) error {
	if len(key) == 0 {
		return errFakeEmptyKey
	}
	for _, p := range []string{"vault/", "txout/", "config/"} {
		if len(key) >= len(p) && string(key[:len(p)]) == p {
			return nil
		}
	}
	return errFakeBadPrefix
}

func (k *storeMigrateKeeperFake) SetRawStoreValue(_ cosmos.Context, key, value []byte) error {
	if err := k.ValidateRawStoreKey(key); err != nil {
		return err
	}
	if len(value) == 0 {
		return errFakeUndecodable // stand-in for "does not decode as the store type"
	}
	k.raw[string(key)] = value
	return nil
}

func (k *storeMigrateKeeperFake) DeleteRawStoreValue(_ cosmos.Context, key []byte) {
	if k.ValidateRawStoreKey(key) != nil {
		return
	}
	delete(k.raw, string(key))
}

var (
	errFakeEmptyKey     = fmtError("empty key")
	errFakeBadPrefix    = fmtError("prefix not allowlisted")
	errFakeUndecodable  = fmtError("undecodable value")
)

type fmtError string

func (e fmtError) Error() string { return string(e) }

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
	if t == nil || t.IsEmpty() {
		return nil
	}
	k.txouts[t.Height] = t
	return nil
}

func (k *storeMigrateKeeperFake) ClearTxOut(_ cosmos.Context, height int64) error {
	delete(k.txouts, height)
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
	bad := []string{"", "FOO:bar", "CONFIG", "VAULTCOIN:onlyone", "TXOUTCANCEL:1", "KVSET:nothex!!", "KVSET", "KVDEL:zz"}
	for _, k := range bad {
		if _, err := parseStoreMigrateTarget(k); err == nil {
			t.Fatalf("expected %q to be rejected", k)
		}
	}
	// KVSET/KVDEL with valid hex keys parse.
	for _, k := range []string{"KVSET:7661756c742f", "KVDEL:7661756c742f"} {
		if _, err := parseStoreMigrateTarget(k); err != nil {
			t.Fatalf("expected %q to parse, got %v", k, err)
		}
	}
}

func TestApplyStoreMigrationKVSetRejectsUndecodable(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := newStoreMigrateKeeperFake()
	vaultKeyHex := hexs("vault/SOMEPUBKEY")

	// Empty value stands in for "does not decode as a Vault" -> rejected, no write.
	if err := applyStoreMigration(ctx, k, "KVSET:"+vaultKeyHex, ""); err == nil {
		t.Fatalf("expected undecodable KVSET to be rejected")
	}
	if len(k.raw) != 0 {
		t.Fatalf("rejected KVSET must not write, got %v", k.raw)
	}

	// Key outside the allowlist is rejected.
	if err := applyStoreMigration(ctx, k, "KVSET:"+hexs("randomprefix/x"), hexs("data")); err == nil {
		t.Fatalf("expected non-allowlisted prefix to be rejected")
	}

	// A valid (allowlisted prefix, non-empty value) write succeeds.
	if err := applyStoreMigration(ctx, k, "KVSET:"+vaultKeyHex, hexs("VAULTBYTES")); err != nil {
		t.Fatalf("expected valid KVSET to apply, got %v", err)
	}
	if string(k.raw["vault/SOMEPUBKEY"]) != "VAULTBYTES" {
		t.Fatalf("expected raw write, got %v", k.raw)
	}
}

func TestApplyStoreMigrationKVDelAllowlist(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := newStoreMigrateKeeperFake()
	k.raw["vault/GONE"] = []byte("x")
	if err := applyStoreMigration(ctx, k, "KVDEL:"+hexs("vault/GONE"), "1"); err != nil {
		t.Fatalf("expected KVDEL to apply, got %v", err)
	}
	if _, ok := k.raw["vault/GONE"]; ok {
		t.Fatalf("expected key deleted")
	}
	// Non-allowlisted prefix rejected.
	if err := applyStoreMigration(ctx, k, "KVDEL:"+hexs("secret/x"), "1"); err == nil {
		t.Fatalf("expected non-allowlisted KVDEL to be rejected")
	}
}

func hexs(s string) string {
	return hex.EncodeToString([]byte(s))
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
