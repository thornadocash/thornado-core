package tss

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"

	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// KeyGen coordinates FROST vault key generation and local keyshare storage.
type KeyGen struct {
	keys       *thornadoclient.Keys
	localState storage.LocalStateManager
	logger     zerolog.Logger
	bridge     thornadoclient.ThornadoBridge
}

func NewTssKeyGen(keys *thornadoclient.Keys, localState storage.LocalStateManager, bridge thornadoclient.ThornadoBridge) (*KeyGen, error) {
	if keys == nil {
		return nil, fmt.Errorf("keys is nil")
	}
	if localState == nil {
		return nil, fmt.Errorf("FROST local state manager is required")
	}
	return &KeyGen{
		keys:       keys,
		localState: localState,
		logger:     log.With().Str("module", "frost_keygen").Logger(),
		bridge:     bridge,
	}, nil
}

func (kg *KeyGen) GenerateNewKey(keygenBlockHeight int64, pKeys common.PubKeys, chains common.Chains) (common.PubKeySet, []types.Blame, error) {
	if len(pKeys) == 0 {
		return common.EmptyPubKeySet, nil, nil
	}
	if !chains.Has(common.BTCChain) {
		return common.EmptyPubKeySet, nil, fmt.Errorf("FROST keygen only supports BTC vaults")
	}

	participants := pKeys.Strings()
	localParty, err := kg.localParty()
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	if !pKeys.Contains(localParty) {
		return common.EmptyPubKeySet, nil, fmt.Errorf("local party %s is not in keygen members", localParty)
	}

	minSigners := frostMinSigners(len(participants))
	shares, pubKeyBytes, err := kg.generateShares(keygenBlockHeight, participants, minSigners)
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	localShare, ok := shares[localParty.String()]
	if !ok {
		return common.EmptyPubKeySet, nil, fmt.Errorf("FROST engine did not return local keyshare")
	}

	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	if err := kg.localState.SaveLocalState(storage.KeygenLocalState{
		PubKey:          vaultPubKey.String(),
		LocalData:       localShare,
		ParticipantKeys: participants,
		LocalPartyKey:   localParty.String(),
		SigningEngine:   storage.SigningEngineFrost,
	}); err != nil {
		return common.EmptyPubKeySet, nil, fmt.Errorf("save FROST local state: %w", err)
	}

	kg.logger.Info().
		Int64("height", keygenBlockHeight).
		Str("pubkey", vaultPubKey.String()).
		Int("members", len(participants)).
		Strs("chains", chains.Strings()).
		Msg("FROST keygen complete")
	return common.NewPubKeySet(vaultPubKey, common.EmptyPubKey), nil, nil
}

type sharedFrostKeygen struct {
	Shares      map[string][]byte `json:"shares"`
	PubKeyBytes []byte            `json:"pub_key_bytes"`
}

func (kg *KeyGen) generateShares(height int64, participants []string, minSigners uint16) (map[string][]byte, []byte, error) {
	dir := strings.TrimSpace(os.Getenv("BIFROST_FROST_SHARED_DEALER_DIR"))
	if dir == "" {
		return frostsessions.KeygenWithThreshold(participants, minSigners)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create shared FROST dealer dir: %w", err)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s", height, minSigners, strings.Join(participants, ","))))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
	lock := path + ".lock"

	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			var out sharedFrostKeygen
			if err := json.Unmarshal(raw, &out); err != nil {
				return nil, nil, fmt.Errorf("read shared FROST keygen: %w", err)
			}
			return out.Shares, out.PubKeyBytes, nil
		}
		fd, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		_ = fd.Close()
		defer os.Remove(lock)

		shares, pubKeyBytes, err := frostsessions.KeygenWithThreshold(participants, minSigners)
		if err != nil {
			return nil, nil, err
		}
		raw, err = json.Marshal(sharedFrostKeygen{Shares: shares, PubKeyBytes: pubKeyBytes})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal shared FROST keygen: %w", err)
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			return nil, nil, fmt.Errorf("write shared FROST keygen: %w", err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return nil, nil, fmt.Errorf("publish shared FROST keygen: %w", err)
		}
		return shares, pubKeyBytes, nil
	}
}

func (kg *KeyGen) localParty() (common.PubKey, error) {
	privKey, err := kg.keys.GetPrivateKey()
	if err != nil {
		return common.EmptyPubKey, err
	}
	return common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(privKey.PubKey().Bytes()))
}

func frostMinSigners(participants int) uint16 {
	if participants <= 0 {
		return 0
	}
	return uint16(math.Ceil(float64(participants) * 2 / 3))
}
