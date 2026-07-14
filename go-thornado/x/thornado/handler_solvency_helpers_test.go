package thornado

import (
	"errors"
	"sort"
	"strconv"
	"testing"

	"cosmossdk.io/log"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type solvencyTestKeeper struct {
	keeper.KVStoreDummy

	configs map[constants.ConfigName]int64
	txOuts  map[int64]*TxOut
	voters  map[common.TxID]ObservedTxVoter
}

func (k solvencyTestKeeper) GetConfigInt64(_ cosmos.Context, key constants.ConfigName) int64 {
	return k.configs[key]
}

func (k solvencyTestKeeper) GetTxOut(_ cosmos.Context, height int64) (*TxOut, error) {
	if txOut, ok := k.txOuts[height]; ok {
		return txOut, nil
	}
	return &TxOut{Height: height}, nil
}

func (k solvencyTestKeeper) GetTxOutIterator(_ cosmos.Context) cosmos.Iterator {
	iter := keeper.NewDummyIterator()
	heights := make([]int64, 0, len(k.txOuts))
	for height := range k.txOuts {
		heights = append(heights, height)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	for _, height := range heights {
		value, _ := k.Cdc().Marshal(k.txOuts[height])
		iter.AddItem([]byte(strconv.FormatInt(height, 10)), value)
	}
	return iter
}

func (k solvencyTestKeeper) GetObservedTxOutVoter(_ cosmos.Context, hash common.TxID) (ObservedTxVoter, error) {
	if voter, ok := k.voters[hash]; ok {
		return voter, nil
	}
	return ObservedTxVoter{}, errors.New("not found")
}

func TestSolvencyCheckAllowsRecentAuthorizedOutboundGasGap(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(315).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	outHash := mustTxID(t, "1EB2B7CBC6D535B8B746AE3111CE8FC2D55483B947DE657C8DCC48E75FC1C32B")
	from := mustAddress(t, "bcrt1p8qsq3qexj7vp0k7ltl2zhlydqhjpz7nhrdluc07gmj9608hah8psckqtka")
	to := mustAddress(t, "bcrt1q9hln7upfz396r6lkry0unnv4y2tuxra6hhvq57")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_226_700))},
	}
	walletCoins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_221_870))}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			309: {
				Height: 309,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
						ToAddress:   to,
						OutHash:     outHash,
						TxType:      types.TxOutTypeOut,
					},
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000)),
						ToAddress:   to,
						OutHash:     outHash,
						TxType:      types.TxOutTypeOut,
					},
				},
			},
		},
		voters: map[common.TxID]ObservedTxVoter{
			outHash: {
				TxID: outHash,
				Tx: common.NewObservedTx(
					common.NewTx(
						outHash,
						from,
						to,
						common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(22_770_000))),
						common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(4_830))},
					),
					313,
					vaultPubKey,
					314,
				),
			},
		},
	}}

	if insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("authorized outbound gas gap should not halt solvency")
	}
}

func TestSolvencyCheckStillHaltsWhenGapExceedsRecentOutboundGas(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(315).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	outHash := mustTxID(t, "1EB2B7CBC6D535B8B746AE3111CE8FC2D55483B947DE657C8DCC48E75FC1C32B")
	from := mustAddress(t, "bcrt1p8qsq3qexj7vp0k7ltl2zhlydqhjpz7nhrdluc07gmj9608hah8psckqtka")
	to := mustAddress(t, "bcrt1q9hln7upfz396r6lkry0unnv4y2tuxra6hhvq57")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_300_000))},
	}
	walletCoins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_221_870))}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			309: {
				Height: 309,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
						ToAddress:   to,
						OutHash:     outHash,
						TxType:      types.TxOutTypeOut,
					},
				},
			},
		},
		voters: map[common.TxID]ObservedTxVoter{
			outHash: {
				TxID: outHash,
				Tx: common.NewObservedTx(
					common.NewTx(
						outHash,
						from,
						to,
						common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000))),
						common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(4_830))},
					),
					313,
					vaultPubKey,
					314,
				),
			},
		},
	}}

	if !insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("gap larger than authorized outbound gas should still halt solvency")
	}
}

func TestSolvencyCheckAllowsOldSignedBTCOutboundAwaitingObservedVoter(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(1886).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	outHash := mustTxID(t, "DEB75C2A44A8A7B01D635E82F7B917E8B5F2B38FC09F91DD719F55BD890970D3")
	from := mustAddress(t, "bcrt1prw2hcce2gnrm22umrrls3fm2vxa73f7tjq3qp63l75nyah6p57ys2a2zp7")
	to := mustAddress(t, "bcrt1qw84my4v866jvy33g92v8008ztplr5c9cd80qsk")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(130_081_548))},
	}
	walletCoins := common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(120_178_454))}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			1600: {
				Height: 1600,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000)),
						ToAddress:   to,
						OutHash:     outHash,
						TxType:      types.TxOutTypeOut,
					},
				},
			},
		},
		voters: map[common.TxID]ObservedTxVoter{
			outHash: {
				TxID: outHash,
				Txs: []common.ObservedTx{
					common.NewObservedTx(
						common.NewTx(
							outHash,
							from,
							to,
							common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(9_900_000))),
							common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_094))},
						),
						1602,
						vaultPubKey,
						1603,
					),
				},
			},
		},
	}}

	if insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("signed outbound awaiting observed voter should not halt solvency")
	}
}

func TestSolvencyCheckAllowsFullBalanceMigrationOpenTxOut(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(315).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	to := mustAddress(t, "bcrt1q9hln7upfz396r6lkry0unnv4y2tuxra6hhvq57")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(12_956_224_454))},
	}
	walletCoins := common.Coins{}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			309: {
				Height: 309,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(12_956_214_454)),
						MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
						ToAddress:   to,
						TxType:      types.TxOutTypeMigrate,
					},
				},
			},
		},
	}}

	if insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("full-balance migration awaiting confirmation should not halt solvency")
	}
}

func TestSolvencyCheckAllowsFullBalanceMigrationAwaitingVaultAccounting(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(1886).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	outHash := mustTxID(t, "DEB75C2A44A8A7B01D635E82F7B917E8B5F2B38FC09F91DD719F55BD890970D3")
	to := mustAddress(t, "bcrt1qw84my4v866jvy33g92v8008ztplr5c9cd80qsk")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(12_956_224_454))},
	}
	walletCoins := common.Coins{}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			1600: {
				Height: 1600,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(12_956_214_454)),
						MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
						ToAddress:   to,
						OutHash:     outHash,
						TxType:      types.TxOutTypeMigrate,
					},
				},
			},
		},
	}}

	if insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("signed full-balance migration awaiting vault accounting should not halt solvency")
	}
}

func TestSolvencyCheckStillHaltsWhenNonMigrateOutboundsZeroVault(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(315).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	to := mustAddress(t, "bcrt1q9hln7upfz396r6lkry0unnv4y2tuxra6hhvq57")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000))},
	}
	walletCoins := common.Coins{}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			309: {
				Height: 309,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000)),
						ToAddress:   to,
						TxType:      types.TxOutTypeOut,
					},
				},
			},
		},
	}}

	if !insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("non-migrate outbounds zeroing the vault should still halt solvency")
	}
}

func TestSolvencyCheckStillHaltsWhenMigrationOnlyPartiallyCoversVault(t *testing.T) {
	ctx := cosmos.Context{}.WithBlockHeight(315).WithLogger(log.NewNopLogger())
	vaultPubKey := GetRandomPubKey()
	to := mustAddress(t, "bcrt1q9hln7upfz396r6lkry0unnv4y2tuxra6hhvq57")

	vault := Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000))},
	}
	walletCoins := common.Coins{}
	mgr := &Mgrs{K: solvencyTestKeeper{
		configs: map[constants.ConfigName]int64{
			constants.Chain_BlockTimeSeconds: 6,
			constants.Keysign_PeriodMinutes:  10,
		},
		txOuts: map[int64]*TxOut{
			309: {
				Height: 309,
				TxArray: []TxOutItem{
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(590_000)),
						MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
						ToAddress:   to,
						TxType:      types.TxOutTypeMigrate,
					},
					{
						Chain:       common.BTCChain,
						VaultPubKey: vaultPubKey,
						Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(400_000)),
						ToAddress:   to,
						TxType:      types.TxOutTypeOut,
					},
				},
			},
		},
	}}

	if !insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain) {
		t.Fatal("migration covering only part of the vault should still halt solvency")
	}
}

func mustTxID(t *testing.T, value string) common.TxID {
	t.Helper()
	txID, err := common.NewTxID(value)
	if err != nil {
		t.Fatal(err)
	}
	return txID
}

func mustAddress(t *testing.T, value string) common.Address {
	t.Helper()
	address, err := common.NewAddress(value)
	if err != nil {
		t.Fatal(err)
	}
	return address
}
