package thornado

import (
	"errors"
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
