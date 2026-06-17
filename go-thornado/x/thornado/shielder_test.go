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

func TestBondShieldCommitmentsAreProtocolGenerated(t *testing.T) {
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
		t.Fatal("bond shield used operator-supplied commitment")
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
	if got := constants.NewConfigValue().GetInt64Value(constants.Withdrawal_FeeMinSats); got != 0 {
		t.Fatalf("withdrawal min fee should be disabled: %d", got)
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
