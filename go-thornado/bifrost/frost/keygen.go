package frost

import (
	"context"
	"fmt"
	"math"
	"sort"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// KeyGen coordinates distributed FROST vault key generation over libp2p.
type KeyGen struct {
	keys        *thornadoclient.Keys
	localState  storage.LocalStateManager
	logger      zerolog.Logger
	bridge      thornadoclient.ThornadoBridge
	coordinator SessionCoordinator
}

func NewFrostKeyGen(
	keys *thornadoclient.Keys,
	localState storage.LocalStateManager,
	bridge thornadoclient.ThornadoBridge,
	coordinator SessionCoordinator,
) (*KeyGen, error) {
	if keys == nil {
		return nil, fmt.Errorf("keys is nil")
	}
	if localState == nil {
		return nil, fmt.Errorf("FROST local state manager is required")
	}
	if coordinator == nil {
		return nil, fmt.Errorf("FROST P2P session coordinator is required")
	}
	return &KeyGen{
		keys:        keys,
		localState:  localState,
		logger:      log.With().Str("module", "frost_keygen").Logger(),
		bridge:      bridge,
		coordinator: coordinator,
	}, nil
}

func (kg *KeyGen) GenerateNewKey(keygenBlockHeight int64, pKeys common.PubKeys, chains common.Chains) (common.PubKeySet, []types.Blame, error) {
	if len(pKeys) == 0 {
		return common.EmptyPubKeySet, nil, nil
	}
	if !chains.Has(common.BTCChain) {
		return common.EmptyPubKeySet, nil, fmt.Errorf("FROST keygen only supports BTC vaults")
	}

	participants := sortedParticipants(pKeys.Strings())
	localParty, err := kg.localParty()
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	if !pKeys.Contains(localParty) {
		return common.EmptyPubKeySet, nil, fmt.Errorf("local party %s is not in keygen members", localParty)
	}

	minSigners := frostMinSigners(len(participants))
	ctx := context.Background()
	localShare, pubKeyBytes, err := kg.coordinator.RunKeygen(ctx, keygenBlockHeight, participants, localParty.String(), minSigners)
	if err != nil {
		return common.EmptyPubKeySet, nil, err
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
		Msg("FROST P2P keygen complete")
	return common.NewPubKeySet(vaultPubKey), nil, nil
}

func (kg *KeyGen) localParty() (common.PubKey, error) {
	privKey, err := kg.keys.GetPrivateKey()
	if err != nil {
		return common.EmptyPubKey, err
	}
	return common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(privKey.PubKey().Bytes()))
}

// FrostMinSigners returns the BFT signing threshold for a vault member set.
func FrostMinSigners(participants int) uint16 {
	return frostMinSigners(participants)
}

func frostMinSigners(participants int) uint16 {
	if participants <= 0 {
		return 0
	}
	return uint16(math.Ceil(float64(participants) * 2 / 3))
}

func sortedParticipantStrings(participants []string) []string {
	out := append([]string(nil), participants...)
	sort.Strings(out)
	return out
}