package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestKeygenSignVerify(t *testing.T) {
	testDealerSignVerify(t, []string{"thorpub-c", "thorpub-a", "thorpub-b"}, 2)
	testDealerSignVerify(t, []string{"a", "b", "c", "d", "e"}, 3)
}

func TestSignWithPathVerifiesOnlyAgainstDerivedKey(t *testing.T) {
	participants := []string{"a", "b", "c"}
	shares, pubKey, err := KeygenWithThreshold(participants, 2)
	if err != nil {
		t.Fatal(err)
	}
	msg := sha256.Sum256([]byte("taproot-path-sign-message"))
	sig, err := SignWithPath(shares[participants[0]], msg[:], 7)
	if err != nil {
		t.Fatal(err)
	}
	derivedPubKey, err := DerivePublicKey(pubKey, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(derivedPubKey, msg[:], sig); err != nil {
		t.Fatal(err)
	}
	if err := Verify(pubKey, msg[:], sig); err == nil {
		t.Fatal("path signature verified against base public key")
	}
}

func testDealerSignVerify(t *testing.T, participants []string, minSigners uint16) {
	t.Helper()
	shares, pubKey, err := KeygenWithThreshold(participants, minSigners)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != len(participants) {
		t.Fatalf("expected %d shares, got %d", len(participants), len(shares))
	}

	msg := sha256.Sum256([]byte("taproot-test-message"))
	sig, err := Sign(shares[participants[0]], msg[:])
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pubKey, msg[:], sig); err != nil {
		t.Fatal(err)
	}
}

func TestSessionKeygenSignVerify(t *testing.T) {
	testSessionSignVerify(t, []string{"thorpub-c", "thorpub-a", "thorpub-b"}, 2)
	testSessionSignVerify(t, []string{"a", "b", "c", "d", "e"}, 3)
}

func TestSessionKeygenSignVerifyAllParticipants(t *testing.T) {
	participants := []string{"thorpub-c", "thorpub-a", "thorpub-f", "thorpub-b", "thorpub-e", "thorpub-d"}
	shares := runKeygenSessions(t, participants, 4)
	first, err := DecodeKeyshare(shares[participants[0]])
	if err != nil {
		t.Fatal(err)
	}
	msg := sha256.Sum256([]byte("taproot-session-all-participants"))
	signers := []string{"thorpub-a", "thorpub-b", "thorpub-c", "thorpub-d", "thorpub-e", "thorpub-f"}
	sigs := runSignSessions(t, signers, shares, msg[:])
	if len(sigs) != len(signers) {
		t.Fatalf("expected %d signatures, got %d", len(signers), len(sigs))
	}
	pubKey, err := hex.DecodeString(first.PublicKeyCompressed)
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range sigs {
		if err := Verify(pubKey, msg[:], sig); err != nil {
			t.Fatal(err)
		}
	}
}

func testSessionSignVerify(t *testing.T, participants []string, minSigners uint16) {
	t.Helper()
	shares := runKeygenSessions(t, participants, minSigners)
	first, err := DecodeKeyshare(shares[participants[0]])
	if err != nil {
		t.Fatal(err)
	}
	msg := sha256.Sum256([]byte("taproot-session-test-message"))
	sigs := runSignSessions(t, participants[:minSigners], shares, msg[:])
	if len(sigs) != int(minSigners) {
		t.Fatalf("expected %d signatures, got %d", minSigners, len(sigs))
	}
	pubKey, err := hex.DecodeString(first.PublicKeyCompressed)
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range sigs {
		if err := Verify(pubKey, msg[:], sig); err != nil {
			t.Fatal(err)
		}
	}
}

func runKeygenSessions(t *testing.T, participants []string, minSigners uint16) map[string][]byte {
	t.Helper()
	handles := make(map[string]Handle)
	for _, p := range participants {
		h, err := KeygenSessionNew(participants, p, minSigners)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = SessionFree(h) }()
		handles[p] = h
	}
	runSessions(t, participants, handles)
	shares := make(map[string][]byte)
	publicKey := ""
	for p, h := range handles {
		share, err := SessionFinish(h)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeKeyshare(share)
		if err != nil {
			t.Fatal(err)
		}
		if publicKey == "" {
			publicKey = decoded.PublicKeyCompressed
		} else if decoded.PublicKeyCompressed != publicKey {
			t.Fatalf("inconsistent distributed keygen public key: %s got %s want %s", p, decoded.PublicKeyCompressed, publicKey)
		}
		shares[p] = share
	}
	return shares
}

func runSignSessions(t *testing.T, participants []string, shares map[string][]byte, msg []byte) map[string][]byte {
	t.Helper()
	handles := make(map[string]Handle)
	for _, p := range participants {
		h, err := SignSessionNew(participants, p, shares[p], msg)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = SessionFree(h) }()
		handles[p] = h
	}
	runSessions(t, participants, handles)
	sigs := make(map[string][]byte)
	for p, h := range handles {
		sig, err := SessionFinish(h)
		if err != nil {
			t.Fatal(err)
		}
		sigs[p] = sig
	}
	return sigs
}

func runSessions(t *testing.T, participants []string, handles map[string]Handle) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		progress := false
		for from, h := range handles {
			for {
				msg, err := SessionOutputMessage(h)
				if err != nil {
					t.Fatal(err)
				}
				if len(msg) == 0 {
					break
				}
				progress = true
				for idx := 0; idx < len(participants); idx++ {
					to, err := SessionMessageReceiver(h, msg, idx)
					if err != nil {
						t.Fatal(err)
					}
					if to == "" {
						break
					}
					target, ok := handles[to]
					if !ok {
						t.Fatalf("unknown receiver %q from %q", to, from)
					}
					if _, err := SessionInputMessage(target, msg); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		if !progress {
			for _, h := range handles {
				if _, err := SessionFinish(h); err != nil {
					goto keepGoing
				}
			}
			return
		}
	keepGoing:
	}
	t.Fatal("sessions did not finish")
}

func TestKeygenParticipantOrderIsDeterministic(t *testing.T) {
	shares, _, err := Keygen([]string{"b", "a", "c"})
	if err != nil {
		t.Fatal(err)
	}
	share, err := DecodeKeyshare(shares["a"])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	for i, item := range want {
		if share.Participants[i] != item {
			t.Fatalf("expected participant %d to be %s, got %s", i, item, share.Participants[i])
		}
	}
}

func TestSignRejectsNonDigestMessage(t *testing.T) {
	shares, _, err := Keygen([]string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(shares["a"], []byte("not-a-32-byte-digest")); err == nil {
		t.Fatal("expected non-32-byte message to fail")
	}
}
