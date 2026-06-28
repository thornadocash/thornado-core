package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common"
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
