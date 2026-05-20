package thorchain

import "testing"

func TestVerifyShielderWithdrawalRejectsMalformedJSON(t *testing.T) {
	err := VerifyShielderWithdrawalJSON([]byte(`not-json`), []byte(`{}`))
	if err == nil {
		t.Fatal("expected malformed proof json to fail")
	}
}

func TestVerifyShielderWithdrawalCallsWrapper(t *testing.T) {
	err := VerifyShielderWithdrawal(ShielderWithdrawalPayload{
		Proof:  []byte(`{}`),
		Public: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected empty shielder proof to fail")
	}
}
