package pubkeymanager

import (
	"sync"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"

	"github.com/thornadocash/go-thornado/common"
)

func TestAddPubKeyRegistersBaseVaultBeforeDepositLookahead(t *testing.T) {
	pk, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		t.Fatal(err)
	}

	pkm := &PubKeyManager{
		rwMutex:        &sync.RWMutex{},
		logger:         zerolog.Nop(),
		callback:       []OnNewPubKey{},
		pathCallback:   []OnNewPubKeyPath{},
		vaultAddresses: map[string]common.ChainVaultInfo{},
	}

	var mu sync.Mutex
	var depositOnce sync.Once
	var order []string
	depositCallback := make(chan struct{})
	firstUserPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	pkm.RegisterCallback(func(common.PubKey) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "base")
		return nil
	})
	pkm.RegisterPathCallback(func(_ common.PubKey, pathIndex uint64) error {
		if pathIndex != firstUserPath {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "first-deposit")
		depositOnce.Do(func() {
			close(depositCallback)
		})
		return nil
	})

	pkm.AddPubKey(pk, true, common.SigningAlgoSecp256k1)
	firstUserAddress, err := common.DeriveBTCTaprootAddress(pk, firstUserPath)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := pkm.IsValidVaultAddress(firstUserAddress.String(), common.BTCChain); !ok {
		t.Fatal("first deposit address was not registered before AddPubKey returned")
	}

	mu.Lock()
	if len(order) < 1 {
		t.Fatalf("expected base callback, got %v", order)
	}
	if order[0] != "base" {
		mu.Unlock()
		t.Fatalf("expected base callback first, got %v", order)
	}
	mu.Unlock()

	select {
	case <-depositCallback:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for deposit lookahead callback")
	}
}

func TestRegisterCallbacksReplayExistingSecpVault(t *testing.T) {
	pk, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		t.Fatal(err)
	}

	pkm := &PubKeyManager{
		rwMutex:        &sync.RWMutex{},
		logger:         zerolog.Nop(),
		callback:       []OnNewPubKey{},
		pathCallback:   []OnNewPubKeyPath{},
		vaultAddresses: map[string]common.ChainVaultInfo{},
		pubkeys: []pubKeyInfo{{
			PubKey: pk,
			Signer: true,
			Algo:   common.SigningAlgoSecp256k1,
		}},
	}

	baseReplay := make(chan struct{})
	pkm.RegisterCallback(func(got common.PubKey) error {
		if got.Equals(pk) {
			close(baseReplay)
		}
		return nil
	})

	select {
	case <-baseReplay:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for base callback replay")
	}

	firstUserPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	if err != nil {
		t.Fatal(err)
	}
	pathReplay := make(chan struct{})
	var pathOnce sync.Once
	pkm.RegisterPathCallback(func(got common.PubKey, pathIndex uint64) error {
		if got.Equals(pk) && pathIndex == firstUserPath {
			pathOnce.Do(func() {
				close(pathReplay)
			})
		}
		return nil
	})

	select {
	case <-pathReplay:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for path callback replay")
	}

	firstUserAddress, err := common.DeriveBTCTaprootAddress(pk, firstUserPath)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := pkm.IsValidVaultAddress(firstUserAddress.String(), common.BTCChain); !ok {
		t.Fatal("existing vault deposit address was not registered during path callback replay")
	}
}
