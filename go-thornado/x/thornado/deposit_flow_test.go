package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestDepositMatchedAfterAddressExpiry(t *testing.T) {
	tests := []struct {
		name    string
		deposit types.DepositRecord
		want    bool
	}{
		{
			name: "active address deposit is not refund eligible",
			deposit: types.DepositRecord{
				MatchedHeight:   99,
				ExpiresAtHeight: 100,
			},
			want: false,
		},
		{
			name: "deposit at expiry height is expired address deposit",
			deposit: types.DepositRecord{
				MatchedHeight:   100,
				ExpiresAtHeight: 100,
			},
			want: true,
		},
		{
			name: "deposit after expiry height is expired address deposit",
			deposit: types.DepositRecord{
				MatchedHeight:   101,
				ExpiresAtHeight: 100,
			},
			want: true,
		},
		{
			name: "missing expiry is not refund eligible",
			deposit: types.DepositRecord{
				MatchedHeight: 100,
			},
			want: false,
		},
		{
			name: "missing matched height is not refund eligible",
			deposit: types.DepositRecord{
				ExpiresAtHeight: 100,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := depositMatchedAfterAddressExpiry(tt.deposit); got != tt.want {
				t.Fatalf("depositMatchedAfterAddressExpiry() = %t, want %t", got, tt.want)
			}
		})
	}
}
