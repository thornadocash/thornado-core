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
	path1Index, err := VaultDepositPathIndex(VaultDepositPathUser, 0, DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if path1Index != 1 {
		t.Fatalf("first vault deposit child path should be 1, got %d", path1Index)
	}
	path2Index, err := VaultDepositPathIndex(VaultDepositPathUser, 1, DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	nodePathIndex, err := VaultDepositPathIndex(VaultDepositPathNode, 0, DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	if nodePathIndex != 1 {
		t.Fatalf("node path type should not alter vault BIP86 child index, got %d", nodePathIndex)
	}
	path1, err := DeriveBTCTaprootAddress(pk, path1Index)
	if err != nil {
		t.Fatal(err)
	}
	path1Again, err := DeriveBTCTaprootAddress(pk, path1Index)
	if err != nil {
		t.Fatal(err)
	}
	path2, err := DeriveBTCTaprootAddress(pk, path2Index)
	if err != nil {
		t.Fatal(err)
	}
	nodePath, err := DeriveBTCTaprootAddress(pk, nodePathIndex)
	if err != nil {
		t.Fatal(err)
	}

	if mainAddr.IsEmpty() || path1.IsEmpty() || path2.IsEmpty() || nodePath.IsEmpty() {
		t.Fatal("derived address is empty")
	}
	if !path1.Equals(path1Again) {
		t.Fatalf("path derivation is not stable: %s != %s", path1, path1Again)
	}
	if mainAddr.Equals(path1) || path1.Equals(path2) {
		t.Fatalf("derived paths must be distinct: main=%s path1=%s path2=%s node=%s", mainAddr, path1, path2, nodePath)
	}
	if !path1.Equals(nodePath) {
		t.Fatalf("same vault child index should derive the same address regardless of deposit type: %s != %s", path1, nodePath)
	}
	if got := UserSecretPath(VaultDepositPathUser, 0, 1); got != "tc84/btc/user/0/1" {
		t.Fatalf("unexpected user secret path: %s", got)
	}
}
