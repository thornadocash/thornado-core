package frost

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	btcschnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"

	"github.com/thornadocash/go-thornado/common"
)

func TestInProcessP2PSignTaprootMainPath(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	allShares, err := RunInProcessKeygenAll(participants, 2)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := frostsessions.DecodeKeyshare(allShares["node-a"])
	if err != nil {
		t.Fatal(err)
	}
	pubKeyBytes, err := hex.DecodeString(decoded.PublicKeyCompressed)
	if err != nil {
		t.Fatal(err)
	}
	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	msg := sha256.Sum256([]byte("taproot-session"))
	sig, err := RunInProcessSign(participants[:2], allShares, "node-a", msg[:], true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tweaked, err := common.DeriveBTCTaprootPubKey(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	parsedPub, err := btcschnorr.ParsePubKey(tweaked)
	if err != nil {
		t.Fatal(err)
	}
	parsedSig, err := btcschnorr.ParseSignature(sig)
	if err != nil {
		t.Fatal(err)
	}
	if !parsedSig.Verify(msg[:], parsedPub) {
		t.Fatal("taproot verify failed for in-process P2P sign")
	}
}