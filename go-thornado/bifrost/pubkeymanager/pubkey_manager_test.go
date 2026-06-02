package pubkeymanager

import (
	"sync"
	"testing"

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
	var order []string
	pkm.RegisterCallback(func(common.PubKey) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "base")
		return nil
	})
	pkm.RegisterPathCallback(func(_ common.PubKey, pathIndex uint64) error {
		firstUserPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
		if err != nil || pathIndex != firstUserPath {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "first-deposit")
		return nil
	})

	pkm.AddPubKey(pk, true, common.SigningAlgoSecp256k1)

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 {
		t.Fatalf("expected base and deposit callbacks, got %v", order)
	}
	if order[0] != "base" {
		t.Fatalf("expected base callback first, got %v", order)
	}
}
