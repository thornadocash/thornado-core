package thornado

import (
	"encoding/json"
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

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

func TestBondSplitCommitmentsAreProtocolGenerated(t *testing.T) {
	rawCommitments := []string{
		`{"denomination_sats":100000,"commitment":"USER_SUPPLIED_A"}`,
		`{"denomination_sats":200000}`,
	}
	notes, err := parseShielderNoteCommitments(rawCommitments, 300000, true)
	if err != nil {
		t.Fatal(err)
	}

	deposit := types.ShielderDeposit{
		DepositID:      GetRandomTxHash(),
		AmountSats:     300000,
		VaultPubKey:    GetRandomPubKey(),
		OperatorPubKey: GetRandomPubKey(),
		NodePubKey:     "thornadovalconspub1addwnpepqwnn92m9kz5mt6q690sy3z9le56d459k5m9twe5k9uc06z6j337ssxq92g5",
		NodeSlot:       3,
	}

	for idx, note := range notes {
		notes[idx].Commitment = shielderBondCommitment(deposit, note.DenominationSats, idx)
	}
	if notes[0].Commitment == "USER_SUPPLIED_A" {
		t.Fatal("bond split used operator-supplied commitment")
	}
	if notes[0].Commitment == "" || notes[1].Commitment == "" || notes[0].Commitment == notes[1].Commitment {
		t.Fatalf("unexpected generated commitments: %#v", notes)
	}
}

func TestUserSplitRequiresCommitments(t *testing.T) {
	payload, err := json.Marshal(shielderNoteCommitment{DenominationSats: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShielderNoteCommitments([]string{string(payload)}, 100000, false); err == nil {
		t.Fatal("expected user split without commitment to fail")
	}
}

func TestWithdrawalFeeIsProtocolTwoPercent(t *testing.T) {
	if got := shielderWithdrawalFeeSatsForBp(100_000_000, WithdrawalFeeBp); got != 2_000_000 {
		t.Fatalf("unexpected fee: %d", got)
	}
}

func TestFeeClaimPayloadBindsRecipientAndCounter(t *testing.T) {
	owner := GetRandomBech32Addr()
	notes := []shielderNoteCommitment{{DenominationSats: 100, Commitment: "ABC"}}
	notePubKeys := []common.PubKey{GetRandomPubKey()}
	payload := shielderFeeClaimPayload("node", owner, 100, 7, notes, notePubKeys)
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 101, 7, notes, notePubKeys)) {
		t.Fatal("payload did not bind accrued amount")
	}
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 100, 8, notes, notePubKeys)) {
		t.Fatal("payload did not bind fee share counter")
	}
	otherNotePubKeys := []common.PubKey{GetRandomPubKey()}
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 100, 7, notes, otherNotePubKeys)) {
		t.Fatal("payload did not bind fee note pubkey")
	}
}
