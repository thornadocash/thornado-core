package main

import (
	"testing"

	cmtcfg "github.com/cometbft/cometbft/config"
)

const testRootMnemonic = "water fatigue garage yard just eight brick recycle flag render wreck sunny six beauty vehicle unfold clap welcome circle year beyond harvest about drastic"

func TestInitializeDerivedNodeValidatorFilesIsDeterministic(t *testing.T) {
	cfgA := cmtcfg.DefaultConfig()
	cfgA.SetRoot(t.TempDir())
	nodeIDA, pubKeyA, err := initializeDerivedNodeValidatorFiles(cfgA, testRootMnemonic, true)
	if err != nil {
		t.Fatalf("initialize first home: %v", err)
	}

	cfgB := cmtcfg.DefaultConfig()
	cfgB.SetRoot(t.TempDir())
	nodeIDB, pubKeyB, err := initializeDerivedNodeValidatorFiles(cfgB, testRootMnemonic, true)
	if err != nil {
		t.Fatalf("initialize second home: %v", err)
	}

	if nodeIDA != nodeIDB {
		t.Fatalf("node ids differ: %s != %s", nodeIDA, nodeIDB)
	}
	if !pubKeyA.Equals(pubKeyB) {
		t.Fatalf("consensus pubkeys differ: %s != %s", pubKeyA.String(), pubKeyB.String())
	}
}

func TestInitializeDerivedNodeValidatorFilesPreservesExistingKeys(t *testing.T) {
	cfg := cmtcfg.DefaultConfig()
	cfg.SetRoot(t.TempDir())
	nodeID, pubKey, err := initializeDerivedNodeValidatorFiles(cfg, testRootMnemonic, true)
	if err != nil {
		t.Fatalf("initialize home: %v", err)
	}

	otherMnemonic := "jelly better achieve collect unaware mountain thought cargo oxygen act hood bridge cloth minimum toddler ginger cage regular advance simple eye dune moral find"
	nodeIDAgain, pubKeyAgain, err := initializeDerivedNodeValidatorFiles(cfg, otherMnemonic, false)
	if err != nil {
		t.Fatalf("reuse home: %v", err)
	}

	if nodeID != nodeIDAgain {
		t.Fatalf("node id changed without overwrite: %s != %s", nodeID, nodeIDAgain)
	}
	if !pubKey.Equals(pubKeyAgain) {
		t.Fatalf("consensus pubkey changed without overwrite: %s != %s", pubKey.String(), pubKeyAgain.String())
	}
}
