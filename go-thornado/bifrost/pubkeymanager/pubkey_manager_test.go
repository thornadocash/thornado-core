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

func TestRegisterPathCallbackDoesNotBlockOnExistingReplay(t *testing.T) {
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

	release := make(chan struct{})
	defer close(release)
	returned := make(chan struct{})
	go func() {
		pkm.RegisterPathCallback(func(common.PubKey, uint64) error {
			<-release
			return nil
		})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("RegisterPathCallback blocked on existing vault path replay")
	}
}

func TestDepositAddressLookaheadDeduplicatesPathTypes(t *testing.T) {
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

	seen := make(map[uint64]struct{})
	pkm.fireDepositAddressLookaheadToCallback(pk, func(_ common.PubKey, pathIndex uint64) error {
		if _, ok := seen[pathIndex]; ok {
			t.Fatalf("duplicate path callback for %d", pathIndex)
		}
		seen[pathIndex] = struct{}{}
		return nil
	})

	if got, want := len(seen), int(common.DepositAddressLookahead); got != want {
		t.Fatalf("unexpected path callback count: got %d want %d", got, want)
	}
}

func TestDepositAddressLookaheadCallbacksSerialized(t *testing.T) {
	pk1, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		t.Fatal(err)
	}
	pk2, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
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
	active := 0
	maxActive := 0
	callback := func(common.PubKey, uint64) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pkm.fireDepositAddressLookaheadToCallback(pk1, callback)
	}()
	go func() {
		defer wg.Done()
		pkm.fireDepositAddressLookaheadToCallback(pk2, callback)
	}()
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("deposit lookahead callbacks ran concurrently: max active %d", maxActive)
	}
}
