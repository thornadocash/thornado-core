package btc

import (
	"context"
	"fmt"
	"sort"
	"sync"

	btcschnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/common"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type frostKeysignBridge interface {
	GetKeysignParty(vaultPubKey common.PubKey) (common.PubKeys, error)
	GetBlockHeight() (int64, error)
}

type frostVaultSigner struct {
	localState  storage.LocalStateManager
	log         zerolog.Logger
	coordinator frost.SessionCoordinator
	bridge      frostKeysignBridge
	localParty  string
	partyLeader string
	partyLeaderMu sync.Mutex
}

func (s *frostVaultSigner) SetPartyLeader(leader string) {
	s.partyLeaderMu.Lock()
	s.partyLeader = leader
	s.partyLeaderMu.Unlock()
}

func (s *frostVaultSigner) ClearPartyLeader() {
	s.partyLeaderMu.Lock()
	s.partyLeader = ""
	s.partyLeaderMu.Unlock()
}

func newVaultSigner(
	bridge frostKeysignBridge,
	localState storage.LocalStateManager,
	log zerolog.Logger,
	coordinator frost.SessionCoordinator,
	localParty string,
) (frost.ThornadoKeyManager, error) {
	if localState == nil {
		return nil, fmt.Errorf("FROST local state manager is required")
	}
	if coordinator == nil {
		return nil, fmt.Errorf("FROST P2P session coordinator is required")
	}
	if localParty == "" {
		return nil, fmt.Errorf("local FROST party key is required")
	}
	return &frostVaultSigner{
		localState:  localState,
		log:         log,
		coordinator: coordinator,
		bridge:      bridge,
		localParty:  localParty,
	}, nil
}

func (s *frostVaultSigner) RemoteSign(msg []byte, algo common.SigningAlgo, vaultPubKey string) ([]byte, []byte, error) {
	return s.RemoteSignWithPath(msg, algo, vaultPubKey, common.MainVaultPathIndex)
}

func (s *frostVaultSigner) RemoteSignWithPath(msg []byte, algo common.SigningAlgo, vaultPubKey string, pathIndex uint64) ([]byte, []byte, error) {
	if algo != common.SigningAlgoSecp256k1 {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("FROST signer only supports secp256k1, got %s", algo),
		})
	}
	state, err := s.localState.GetLocalState(vaultPubKey)
	if err != nil {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}
	if state.Engine() != storage.SigningEngineFrost {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("vault %s is not a FROST keyshare", vaultPubKey),
		})
	}
	if len(state.LocalData) == 0 {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("vault %s has empty FROST keyshare data", vaultPubKey),
		})
	}

	pubKey, err := common.NewPubKey(vaultPubKey)
	if err != nil {
		return nil, nil, err
	}
	var childTweak []byte
	if pathIndex != common.MainVaultPathIndex {
		childTweak, err = common.VaultPathTweakRoot(pubKey, pathIndex)
		if err != nil {
			return nil, nil, err
		}
	}

	signingParty := s.localParty
	if state.LocalPartyKey != "" {
		signingParty = state.LocalPartyKey
	}
	participants := append([]string(nil), state.ParticipantKeys...)
	if len(participants) == 0 {
		vaultKey, err := common.NewPubKey(vaultPubKey)
		if err != nil {
			return nil, nil, err
		}
		party, err := s.bridge.GetKeysignParty(vaultKey)
		if err != nil {
			return nil, nil, frost.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
		}
		participants = party.Strings()
	}
	sort.Strings(participants)
	height, err := s.bridge.GetBlockHeight()
	if err != nil {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}

	sessionID := frost.SignSessionID(vaultPubKey, msg)
	s.partyLeaderMu.Lock()
	partyLeader := s.partyLeader
	s.partyLeaderMu.Unlock()
	signature, err := s.coordinator.RunSign(
		context.Background(),
		sessionID,
		height,
		participants,
		signingParty,
		state.LocalData,
		msg,
		true,
		nil,
		childTweak,
		partyLeader,
	)
	if err != nil {
		return nil, nil, frost.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}
	if err := verifyTaprootSignature(pubKey, pathIndex, msg, signature); err != nil {
		return nil, nil, err
	}

	s.log.Debug().Str("vault_pub_key", vaultPubKey).Msg("created FROST P2P vault signature")
	return signature, nil, nil
}

func (s *frostVaultSigner) LocalStateEngine(vaultPubKey string) string {
	state, err := s.localState.GetLocalState(vaultPubKey)
	if err != nil {
		return ""
	}
	return state.Engine()
}

func verifyTaprootSignature(pubKey common.PubKey, pathIndex uint64, msg, signature []byte) error {
	tweakedPubKey, err := common.DeriveBTCTaprootPubKey(pubKey, pathIndex)
	if err != nil {
		return err
	}
	parsedPubKey, err := btcschnorr.ParsePubKey(tweakedPubKey)
	if err != nil {
		return err
	}
	parsedSignature, err := btcschnorr.ParseSignature(signature)
	if err != nil {
		return err
	}
	if !parsedSignature.Verify(msg, parsedPubKey) {
		return fmt.Errorf("BIP340 signature verification failed for path index %d", pathIndex)
	}
	return nil
}

func (s *frostVaultSigner) GetPrivKey() crypto.PrivKey {
	return nil
}

func (s *frostVaultSigner) GetAddr() sdk.AccAddress {
	return nil
}

func (s *frostVaultSigner) ExportAsMnemonic() (string, error) {
	return "", fmt.Errorf("FROST vault signer does not expose mnemonic export")
}

func (s *frostVaultSigner) ExportAsPrivateKey() (string, error) {
	return "", fmt.Errorf("FROST vault signer does not expose private key export")
}

func (s *frostVaultSigner) ExportAsKeyStore(password string) (*frost.EncryptedKeyJSON, error) {
	return nil, fmt.Errorf("FROST vault signer does not expose keystore export")
}

func (s *frostVaultSigner) Start() {}
func (s *frostVaultSigner) Stop()  {}