package thornado

import (
	"encoding/json"
	"testing"

	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestVerifyShielderRedeemRejectsMalformedJSON(t *testing.T) {
	err := VerifyShielderRedeemJSON([]byte(`not-json`), []byte(`{}`))
	if err == nil {
		t.Fatal("expected malformed proof json to fail")
	}
}

func TestVerifyShielderRedeemCallsWrapper(t *testing.T) {
	err := VerifyShielderRedeem(ShielderRedeemPayload{
		Proof:  []byte(`{}`),
		Public: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected empty shielder proof to fail")
	}
}

func TestBondSplitCommitmentsAreProtocolGenerated(t *testing.T) {
	rawCommitments := []string{
		`{"denomination_sats":1000000,"commitment":"USER_SUPPLIED_A"}`,
		`{"denomination_sats":1000000}`,
	}
	notes, err := parseShielderNoteCommitments(rawCommitments, 2000000, true)
	if err != nil {
		t.Fatal(err)
	}

	deposit := types.DepositRecord{
		DepositID:      GetRandomTxHash(),
		AmountSats:     2000000,
		VaultPubKey:    GetRandomPubKey(),
		OperatorPubKey: GetRandomPubKey(),
		NodePubKey:     "thornadovalconspub1addwnpepqwnn92m9kz5mt6q690sy3z9le56d459k5m9twe5k9uc06z6j337ssxq92g5",
		NodeSlot:       3,
	}

	for idx, note := range notes {
		commitment, err := shielderBondCommitment(deposit, note.DenominationSats, idx)
		if err != nil {
			t.Fatal(err)
		}
		notes[idx].Commitment = commitment
	}
	if notes[0].Commitment == "USER_SUPPLIED_A" {
		t.Fatal("bond split used operator-supplied commitment")
	}
	if notes[0].Commitment == "" || notes[1].Commitment == "" || notes[0].Commitment == notes[1].Commitment {
		t.Fatalf("unexpected generated commitments: %#v", notes)
	}
	root, err := ComputeShielderMerkleRoot([]string{notes[0].Commitment, notes[1].Commitment})
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		t.Fatal("expected bond commitments to produce a shielder merkle root")
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

func TestWithdrawalFeeIsProtocolOnePercent(t *testing.T) {
	if got := withdrawalFeeSatsForBp(100_000_000, uint64(constants.NewConfigValue().GetInt64Value(constants.Withdrawal_FeeBasisPoints))); got != 1_000_000 {
		t.Fatalf("unexpected fee: %d", got)
	}
}

func TestFeeClaimPayloadBindsRecipientAndCounter(t *testing.T) {
	owner := GetRandomBech32Addr()
	notes := []shielderNoteCommitment{{DenominationSats: 100, Commitment: "ABC"}}
	notePubKeys := []string{"02b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"}
	payload := shielderFeeClaimPayload("node", owner, 100, 7, notes, notePubKeys)
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 101, 7, notes, notePubKeys)) {
		t.Fatal("payload did not bind accrued amount")
	}
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 100, 8, notes, notePubKeys)) {
		t.Fatal("payload did not bind fee share counter")
	}
	otherNotePubKeys := []string{"03b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3"}
	if string(payload) == string(shielderFeeClaimPayload("node", owner, 100, 7, notes, otherNotePubKeys)) {
		t.Fatal("payload did not bind fee note pubkey")
	}
}
