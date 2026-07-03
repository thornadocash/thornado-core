package thornado

import (
	"testing"

	"cosmossdk.io/log"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

type vaultDebitRepairKeeperFake struct {
	configs      map[string]int64
	vaults       Vaults
	saved        []Vault
	routes       []common.InvariantRoute
	votesCleared []string
}

func (k *vaultDebitRepairKeeperFake) GetConfig(_ cosmos.Context, key string) (int64, error) {
	return k.configs[key], nil
}

func (k *vaultDebitRepairKeeperFake) SetConfig(_ cosmos.Context, key string, value int64) {
	k.configs[key] = value
}

func (k *vaultDebitRepairKeeperFake) DeleteNodeConfigs(_ cosmos.Context, key string) {
	k.votesCleared = append(k.votesCleared, key)
}

func (k *vaultDebitRepairKeeperFake) GetBaseVaultsByStatus(_ cosmos.Context, status VaultStatus) (Vaults, error) {
	out := Vaults{}
	for _, v := range k.vaults {
		if v.Status == status {
			out = append(out, v)
		}
	}
	return out, nil
}

func (k *vaultDebitRepairKeeperFake) SetVault(_ cosmos.Context, vault Vault) error {
	k.saved = append(k.saved, vault)
	for i := range k.vaults {
		if k.vaults[i].PubKey.Equals(vault.PubKey) {
			k.vaults[i] = vault
		}
	}
	return nil
}

func (k *vaultDebitRepairKeeperFake) InvariantRoutes() []common.InvariantRoute {
	return k.routes
}

func retiringTestVault(sats uint64) Vault {
	return Vault{
		PubKey: common.PubKey("tthorpub1testretiringvault"),
		Status: RetiringVault,
		Coins:  common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(sats))),
	}
}

func TestRetiringVaultDebitRepairApplies(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := &vaultDebitRepairKeeperFake{
		configs: map[string]int64{RepairRetiringVaultDebitSatsKey: 49_500_000},
		vaults:  Vaults{retiringTestVault(12_294_406_454)},
		routes: []common.InvariantRoute{
			common.NewInvariantRoute("vault_backing", func(cosmos.Context) ([]string, bool) {
				return nil, false
			}),
		},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultDebitSatsKey, false)

	if got := k.configs[RepairRetiringVaultDebitSatsKey]; got != 0 {
		t.Fatalf("expected repair config cleared, got %d", got)
	}
	got := k.vaults[0].Coins.GetCoin(common.BTCAsset).Amount.Uint64()
	if got != 12_244_906_454 {
		t.Fatalf("expected book 12244906454 after debit, got %d", got)
	}
	if len(k.votesCleared) != 1 || k.votesCleared[0] != RepairRetiringVaultDebitSatsKey {
		t.Fatalf("expected node votes cleared for debit key, got %v", k.votesCleared)
	}
}

func TestRetiringVaultCreditRepairApplies(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := &vaultDebitRepairKeeperFake{
		configs: map[string]int64{RepairRetiringVaultCreditSatsKey: 49_500_000},
		vaults:  Vaults{retiringTestVault(12_195_406_454)},
		routes: []common.InvariantRoute{
			common.NewInvariantRoute("vault_backing", func(cosmos.Context) ([]string, bool) {
				return nil, false
			}),
		},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultCreditSatsKey, true)

	got := k.vaults[0].Coins.GetCoin(common.BTCAsset).Amount.Uint64()
	if got != 12_244_906_454 {
		t.Fatalf("expected book 12244906454 after credit, got %d", got)
	}
	if len(k.votesCleared) != 1 || k.votesCleared[0] != RepairRetiringVaultCreditSatsKey {
		t.Fatalf("expected node votes cleared for credit key, got %v", k.votesCleared)
	}
}

func TestRetiringVaultDebitRepairRevertsOnBrokenInvariant(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())
	k := &vaultDebitRepairKeeperFake{
		configs: map[string]int64{RepairRetiringVaultDebitSatsKey: 1000},
		vaults:  Vaults{retiringTestVault(5000)},
		routes: []common.InvariantRoute{
			common.NewInvariantRoute("vault_backing", func(cosmos.Context) ([]string, bool) {
				return []string{"deficit"}, true
			}),
		},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultDebitSatsKey, false)

	got := k.vaults[0].Coins.GetCoin(common.BTCAsset).Amount.Uint64()
	if got != 5000 {
		t.Fatalf("expected book restored to 5000 after invariant break, got %d", got)
	}
	if got := k.configs[RepairRetiringVaultDebitSatsKey]; got != 0 {
		t.Fatalf("expected repair config cleared even on revert, got %d", got)
	}
}

func TestRetiringVaultDebitRepairGuards(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(100).WithLogger(log.NewNopLogger())

	// No-op when config unset.
	k := &vaultDebitRepairKeeperFake{
		configs: map[string]int64{},
		vaults:  Vaults{retiringTestVault(5000)},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultDebitSatsKey, false)
	if len(k.saved) != 0 {
		t.Fatal("expected no vault writes when config unset")
	}

	// Refuses a debit larger than the book.
	k = &vaultDebitRepairKeeperFake{
		configs: map[string]int64{RepairRetiringVaultDebitSatsKey: 10_000},
		vaults:  Vaults{retiringTestVault(5000)},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultDebitSatsKey, false)
	if len(k.saved) != 0 {
		t.Fatal("expected no vault writes when debit exceeds book")
	}

	// Refuses when there are two retiring vaults.
	k = &vaultDebitRepairKeeperFake{
		configs: map[string]int64{RepairRetiringVaultDebitSatsKey: 10},
		vaults:  Vaults{retiringTestVault(5000), retiringTestVault(6000)},
	}
	applyVotedRetiringVaultRepair(ctx, k, &invariantHaltEventMgr{}, RepairRetiringVaultDebitSatsKey, false)
	if len(k.saved) != 0 {
		t.Fatal("expected no vault writes with two retiring vaults")
	}
}
