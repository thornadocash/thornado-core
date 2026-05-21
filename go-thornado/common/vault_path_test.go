package common

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestDeriveBTCTaprootAddress(t *testing.T) {
	_, pubKey, _ := testdata.KeyTestPubAddr()
	spk, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := NewPubKey(spk)
	if err != nil {
		t.Fatal(err)
	}

	mainAddr, err := DeriveBTCTaprootAddress(pk, MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	path1, err := DeriveBTCTaprootAddress(pk, FirstDepositPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	path1Again, err := DeriveBTCTaprootAddress(pk, FirstDepositPathIndex)
	if err != nil {
		t.Fatal(err)
	}
	path2, err := DeriveBTCTaprootAddress(pk, FirstDepositPathIndex+1)
	if err != nil {
		t.Fatal(err)
	}

	if mainAddr.IsEmpty() || path1.IsEmpty() || path2.IsEmpty() {
		t.Fatal("derived address is empty")
	}
	if !path1.Equals(path1Again) {
		t.Fatalf("path derivation is not stable: %s != %s", path1, path1Again)
	}
	if mainAddr.Equals(path1) || path1.Equals(path2) {
		t.Fatalf("derived paths must be distinct: main=%s path1=%s path2=%s", mainAddr, path1, path2)
	}
}
