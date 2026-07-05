package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestDepositMatchedAfterAddressExpiryRequiresVaultChurn(t *testing.T) {
	ctx := flowTestContext()
	activeVault := GetRandomPubKey()
	replacementVault := GetRandomPubKey()

	tests := []struct {
		name    string
		deposit types.DepositRecord
		active  []common.PubKey
		want    bool
	}{
		{
			name: "active address deposit is not refund eligible",
			deposit: types.DepositRecord{
				MatchedHeight:   99,
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			active: []common.PubKey{activeVault},
			want:   false,
		},
		{
			name: "deposit at expiry height is still active without vault churn",
			deposit: types.DepositRecord{
				MatchedHeight:   100,
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			active: []common.PubKey{activeVault},
			want:   false,
		},
		{
			name: "deposit after expiry height is still active without vault churn",
			deposit: types.DepositRecord{
				MatchedHeight:   101,
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			active: []common.PubKey{activeVault},
			want:   false,
		},
		{
			name: "deposit after expiry height is expired after base vault churn",
			deposit: types.DepositRecord{
				MatchedHeight:   101,
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			active: []common.PubKey{replacementVault},
			want:   true,
		},
		{
			name: "missing expiry is not refund eligible",
			deposit: types.DepositRecord{
				MatchedHeight: 100,
				VaultPubKey:   activeVault,
			},
			active: []common.PubKey{replacementVault},
			want:   false,
		},
		{
			name: "missing matched height is not refund eligible",
			deposit: types.DepositRecord{
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			active: []common.PubKey{replacementVault},
			want:   false,
		},
		{
			name: "missing active vault set is not refund eligible",
			deposit: types.DepositRecord{
				MatchedHeight:   101,
				ExpiresAtHeight: 100,
				VaultPubKey:     activeVault,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newShielderFlowTestKeeper()
			for _, pubKey := range tt.active {
				if err := k.SetVault(ctx, Vault{PubKey: pubKey, Status: ActiveVault, Type: BaseVault}); err != nil {
					t.Fatal(err)
				}
			}
			if got := depositMatchedAfterAddressExpiry(ctx, k, tt.deposit); got != tt.want {
				t.Fatalf("depositMatchedAfterAddressExpiry() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNextDepositAddressExpiryHeightUsesNextChurnOnly(t *testing.T) {
	ctx := flowTestContext().WithBlockHeight(99)
	k := newShielderFlowTestKeeper()
	k.configs[constants.Chain_BlockTimeSeconds] = 6
	k.configs[constants.Churn_IntervalMinutes] = 10

	if got, want := nextDepositAddressExpiryHeight(ctx, k), int64(100); got != want {
		t.Fatalf("nextDepositAddressExpiryHeight() = %d, want next churn height %d", got, want)
	}
}

func TestMatchCoreDepositProcessesMultipleOutputsFromSameBTCTx(t *testing.T) {
	SetupConfigForTest()
	ctx := testContext(200)
	k := newShielderFlowTestKeeper()
	mgr := newShielderFlowTestManager(k)

	vaultPubKey := GetRandomPubKey()
	vault := NewVaultV2(10, ActiveVault, BaseVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	if err := k.SetVault(ctx, vault); err != nil {
		t.Fatal(err)
	}

	firstPath := testUserDepositPathIndex(t, 1)
	secondPath := testUserDepositPathIndex(t, 2)
	firstAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	ownerA := cosmos.AccAddress("owner-a")
	ownerB := cosmos.AccAddress("owner-b")
	if err := k.SetDepositAddress(ctx, types.DepositAddress{
		Owner:       ownerA,
		Address:     firstAddress,
		VaultPubKey: vaultPubKey,
		PathIndex:   firstPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := k.SetDepositAddress(ctx, types.DepositAddress{
		Owner:       ownerB,
		Address:     secondAddress,
		VaultPubKey: vaultPubKey,
		PathIndex:   secondPath,
	}); err != nil {
		t.Fatal(err)
	}

	txID := GetRandomTxHash()
	sender := GetRandomBTCAddress()
	firstTx := common.NewTx(
		txID,
		sender,
		firstAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(20_000_000))),
		common.Gas{},
	)
	firstTx.SourceVout = 2
	secondTx := common.NewTx(
		txID,
		sender,
		secondAddress,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(30_000_000))),
		common.Gas{},
	)
	secondTx.SourceVout = 4

	firstDeposit, err := MatchCoreDeposit(ctx, mgr, common.NewObservedTx(firstTx, 100, vaultPubKey, 100))
	if err != nil {
		t.Fatal(err)
	}
	secondDeposit, err := MatchCoreDeposit(ctx, mgr, common.NewObservedTx(secondTx, 100, vaultPubKey, 100))
	if err != nil {
		t.Fatal(err)
	}
	if firstDeposit.DepositID.Equals(secondDeposit.DepositID) {
		t.Fatalf("deposits used the same id: %s", firstDeposit.DepositID.String())
	}
	if !firstDeposit.InboundTxID.Equals(txID) || firstDeposit.SourceVout != 2 {
		t.Fatalf("first deposit lost source outpoint: %#v", firstDeposit)
	}
	if !secondDeposit.InboundTxID.Equals(txID) || secondDeposit.SourceVout != 4 {
		t.Fatalf("second deposit lost source outpoint: %#v", secondDeposit)
	}
	if len(k.txOuts) != 2 {
		t.Fatalf("expected two sweep txouts, got %#v", k.txOuts)
	}
	// Batched sweeps carry the epoch union; each item's own outpoint stays
	// recoverable via its path stamp.
	assertPin := func(item TxOutItem, depositID common.TxID, wantVout uint32) {
		t.Helper()
		if !item.InHash.Equals(depositID) {
			t.Fatalf("sweep in_hash mismatch: %#v", item)
		}
		pins := btcItemPinnedInputs(item)
		if len(pins) != 1 || !pins[0].TxId.Equals(txID) || pins[0].Vout != wantVout {
			t.Fatalf("sweep did not preserve source outpoint %d: pins=%#v item=%#v", wantVout, pins, item)
		}
	}
	assertPin(k.txOuts[0], firstDeposit.DepositID, 2)
	assertPin(k.txOuts[1], secondDeposit.DepositID, 4)
}
