package thornado

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type shielderFloorTestKeeper struct {
	keeper.KVStoreDummy
	feePool     types.FeePool
	commitments map[string]bool
}

func (shielderFloorTestKeeper) GetConfigInt64(_ cosmos.Context, key constants.ConfigName) int64 {
	return constants.NewConfigValue().GetInt64Value(key)
}

func (k shielderFloorTestKeeper) GetFeePool(_ cosmos.Context) (types.FeePool, error) {
	return k.feePool, nil
}

func (k *shielderFloorTestKeeper) SetFeePool(_ cosmos.Context, pool types.FeePool) error {
	k.feePool = pool
	return nil
}

func (k *shielderFloorTestKeeper) ShielderCommitmentExists(_ cosmos.Context, commitment string) bool {
	if k.commitments == nil {
		return false
	}
	return k.commitments[commitment]
}

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

func TestDepositRequestPowAllowsUserDepositsWithoutShieldNotes(t *testing.T) {
	msg := types.NewMsgDepositRequestPow("pow-token", strings.Repeat("02", 33))
	if err := msg.ValidateBasic(); err != nil {
		t.Fatalf("expected user deposit without note commitments to pass: %v", err)
	}
}

func TestRejectLeakyShielderRedeemProofRejectsPrivateFields(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"nullifier":"nf",
		"secret":"",
		"commitment":"",
		"merkle_root":"root"
	}`))
	if err == nil {
		t.Fatal("expected proof carrying raw nullifier to fail")
	}
}

func TestRejectLeakyShielderRedeemProofRejectsTornadoMerklePath(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"nullifier":"",
		"secret":"",
		"commitment":"",
		"merkle_root":"root",
		"tornado":{"merkle_path":{"path_elements":["abc"]}}
	}`))
	if err == nil {
		t.Fatal("expected tornado merkle path leak to fail")
	}
}

func TestRejectLeakyShielderRedeemProofAcceptsLegitimateProof(t *testing.T) {
	legit := []byte(`{
		"merkle_root":"12345",
		"tornado":{
			"protocol":"tornado-withdraw-v1",
			"groth16":{
				"pi_a":["1","2","1"],
				"pi_b":[["3","4"],["5","6"],["1","0"]],
				"pi_c":["7","8","1"],
				"protocol":"groth16"
			}
		}
	}`)
	if err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, legit); err != nil {
		t.Fatalf("legitimate proof shape should pass allowlist: %v", err)
	}
}

func TestRejectLeakyShielderRedeemProofRejectsPathIndices(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"merkle_root":"12345",
		"tornado":{"protocol":"p","merkle_path":{"path_indices":[0,1]}}
	}`))
	if err == nil {
		t.Fatal("expected tornado path_indices leak to fail")
	}
}

func TestRejectLeakyShielderRedeemProofRejectsPathElements(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"merkle_root":"12345",
		"tornado":{"protocol":"p","merkle_path":{"path_elements":["abc"]}}
	}`))
	if err == nil {
		t.Fatal("expected tornado path_elements leak to fail")
	}
}

func TestRejectLeakyShielderRedeemProofRejectsLeafIndex(t *testing.T) {
	for _, key := range []string{"leaf_index", "index", "position", "deposit_id", "depositIndex"} {
		err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
			"merkle_root":"12345",
			"`+key+`":7
		}`))
		if err == nil {
			t.Fatalf("expected leaked field %q to fail", key)
		}
	}
}

func TestRejectLeakyShielderRedeemProofRejectsUnknownKey(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"merkle_root":"12345",
		"totally_unexpected":"x"
	}`))
	if err == nil {
		t.Fatal("expected arbitrary unknown key to fail")
	}
}

func TestRejectLeakyShielderRedeemProofRejectsNestedUnknownKey(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"merkle_root":"12345",
		"tornado":{"protocol":"p","groth16":{"pi_a":["1"],"pi_b":[["1"]],"pi_c":["1"],"leaked":"x"}}
	}`))
	if err == nil {
		t.Fatal("expected nested unknown groth16 key to fail")
	}
}

func TestRejectLeakyShielderRedeemProofRejectsRawNullifier(t *testing.T) {
	err := RejectLeakyShielderRedeemProof(cosmos.Context{}, &shielderFloorTestKeeper{}, []byte(`{
		"merkle_root":"12345",
		"nullifier":"nf"
	}`))
	if err == nil {
		t.Fatal("expected proof carrying raw nullifier to fail")
	}
}

func TestValidateShielderRedeemPublicJSONRejectsMissingRecipient(t *testing.T) {
	err := ValidateShielderRedeemPublicJSON([]byte(`{
		"nullifier_hash":"1",
		"merkle_root":"2",
		"denomination_sats":100000,
		"fee_sats":1000
	}`))
	if err == nil {
		t.Fatal("expected missing recipient to fail")
	}
}

func TestUserShieldRequiresCommitments(t *testing.T) {
	payload, err := json.Marshal(shielderNoteCommitment{DenominationSats: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseShielderNoteCommitments([]string{string(payload)}, 100000, false); err == nil {
		t.Fatal("expected user shield without commitment to fail")
	}
}

func TestShieldDustRemainderGoesToFeeFloor(t *testing.T) {
	ctx := cosmos.Context{}
	k := &shielderFloorTestKeeper{}
	minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))
	notes := []shielderNoteCommitment{
		{DenominationSats: minNote, Commitment: "NOTE_A"},
		{DenominationSats: minNote - 1, Commitment: "DUST_NOTE"},
	}

	filtered, remainder, err := applyShielderNoteFloor(ctx, k, notes, minNote*2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Commitment != "NOTE_A" {
		t.Fatalf("unexpected filtered notes: %#v", filtered)
	}
	if remainder != minNote {
		t.Fatalf("expected dust note plus unallocated remainder to go to fees: %d", remainder)
	}
}

func TestProtocolGeneratedDustNoteGoesToFeeFloor(t *testing.T) {
	ctx := cosmos.Context{}
	k := &shielderFloorTestKeeper{}
	minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))

	remainder, err := shielderDustRemainder(ctx, k, minNote-1)
	if err != nil {
		t.Fatal(err)
	}
	if remainder != minNote-1 {
		t.Fatalf("expected protocol dust note to go to fees: %d", remainder)
	}
	remainder, err = shielderDustRemainder(ctx, k, minNote)
	if err != nil {
		t.Fatal(err)
	}
	if remainder != 0 {
		t.Fatalf("expected minimum-sized note to remain spendable: %d", remainder)
	}
}

func TestFeePoolWaitsUntilEveryNodeGetsMinimumNote(t *testing.T) {
	ctx := cosmos.Context{}
	k := &shielderFloorTestKeeper{}
	minNote := uint64(k.GetConfigInt64(ctx, constants.Shielder_NoteAmountMinSats))

	if err := setDistributedFeePool(ctx, k, types.FeePool{
		PendingSats: minNote*3 - 1,
		TotalSlots:  3,
	}); err != nil {
		t.Fatal(err)
	}
	if k.feePool.FeePerSlotShare != 0 || k.feePool.PendingSats != minNote*3-1 {
		t.Fatalf("fee pool distributed before every node could claim a min note: %#v", k.feePool)
	}

	if err := setDistributedFeePool(ctx, k, types.FeePool{
		PendingSats: minNote * 3,
		TotalSlots:  3,
	}); err != nil {
		t.Fatal(err)
	}
	if k.feePool.FeePerSlotShare != minNote || k.feePool.PendingSats != 0 {
		t.Fatalf("fee pool did not distribute at min note threshold: %#v", k.feePool)
	}
}

func TestStoredShielderNoteRecordIsPublicOnly(t *testing.T) {
	record := types.StoredShielderNoteRecord{
		Commitment:       "COMMITMENT",
		DenominationSats: 100000,
	}
	if err := record.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestWithdrawalFeeIsProtocolOnePercent(t *testing.T) {
	if got := withdrawalFeeSatsForBp(100_000_000, uint64(constants.NewConfigValue().GetInt64Value(constants.Withdrawal_FeeBasisPoints))); got != 1_000_000 {
		t.Fatalf("unexpected fee: %d", got)
	}
	k := newShielderFlowTestKeeper()
	ctx := cosmos.Context{}
	if got := withdrawalFeeSats(ctx, k, 1_000_000); got != 10_000 {
		t.Fatalf("0.01 BTC note fee should be exactly 1%%: %d", got)
	}
	if got := withdrawalFeeSats(ctx, k, 100_000_000); got != 1_000_000 {
		t.Fatalf("1 BTC note fee should be exactly 1%%: %d", got)
	}
	if err := validateShielderRedeemPublicFee(types.ShielderRedeemPolicyUserBTC, shielderRedeemPublicInputs{
		DenominationSats: 1_000_000,
		FeeSats:          0,
	}); err != nil {
		t.Fatalf("user withdrawal public fee should not be protocol-selected: %v", err)
	}
	if err := validateShielderRedeemPublicFee(types.ShielderRedeemPolicyUserBTC, shielderRedeemPublicInputs{
		DenominationSats: 1_000_000,
		FeeSats:          123,
	}); err != nil {
		t.Fatalf("user withdrawal public fee should be ignored by protocol: %v", err)
	}
	if err := validateShielderRedeemPublicFee(types.ShielderRedeemPolicyBondEscrow, shielderRedeemPublicInputs{
		DenominationSats: 1_000_000,
		FeeSats:          1,
	}); err == nil {
		t.Fatal("bond escrow public fee should still be rejected")
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
