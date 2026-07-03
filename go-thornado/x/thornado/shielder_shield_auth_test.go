package thornado

import "testing"

// Fixture captured from a live shield authorization: the client signed the
// digest over the lower-case deposit id (the case the sync/API returns). The
// ante verifies with depositID.String(), which NewTxID upper-cases — so the
// verifier must accept the signature over either case.
const (
	shieldAuthPubkey    = "020fd50f7ac5a97f426d247d1c284014b35745999af66568c1de380089a735acb0"
	shieldAuthDepositID = "ea2773d657ff1d39530380242b1e7caa3577e0e7f93aa29fe350b2bef4b44db9"
	shieldAuthSignature = "3044022059e616edae9a2f36dacb50cb90b7f75db31982ac82ce60075b2f51795f0c76c502203983f0311081374d80ec8fc23c305af5c4dc7beee9dbe8172618bd408888a366"
	shieldAuthAmount    = uint64(20000000)
)

func shieldAuthCommitments() []string {
	return []string{
		`{"denomination_sats":10000000,"owner_pubkey":"","signature":"","commitment":"6bd5d96ab82916d9acfd04507170c389d822ec1a03c6e6006ed456377f77ff27"}`,
		`{"denomination_sats":10000000,"owner_pubkey":"","signature":"","commitment":"a6672d6cef97d220a8a9fcf0991bceada74737dd77b448d1817b9ac8dac91e06"}`,
	}
}

func TestVerifyShieldAuthorizationAcceptsLowerCaseDepositID(t *testing.T) {
	if err := VerifyShieldAuthorization(shieldAuthPubkey, shieldAuthSignature, shieldAuthDepositID, shieldAuthAmount, shieldAuthCommitments()); err != nil {
		t.Fatalf("lower-case deposit id (msg_server path) must verify: %v", err)
	}
}

func TestVerifyShieldAuthorizationAcceptsUpperCaseDepositID(t *testing.T) {
	upper := "EA2773D657FF1D39530380242B1E7CAA3577E0E7F93AA29FE350B2BEF4B44DB9"
	if err := VerifyShieldAuthorization(shieldAuthPubkey, shieldAuthSignature, upper, shieldAuthAmount, shieldAuthCommitments()); err != nil {
		t.Fatalf("upper-case deposit id (ante path, NewTxID upper-cases) must verify: %v", err)
	}
}

func TestVerifyShieldAuthorizationRejectsWrongAmount(t *testing.T) {
	if err := VerifyShieldAuthorization(shieldAuthPubkey, shieldAuthSignature, shieldAuthDepositID, shieldAuthAmount+1, shieldAuthCommitments()); err == nil {
		t.Fatal("a signature over a different amount must not verify")
	}
}
