package tss

import (
	"fmt"
	"math"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	frostsessions "github.com/thornadocash/go-thornado/frost/go-wrappers/go-frost/sessions"

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

	shares, pubKeyBytes, err := frostsessions.KeygenWithThreshold(participants, frostMinSigners(len(participants)))
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	localShare, ok := shares[localParty.String()]
	if !ok {
		return common.EmptyPubKeySet, nil, fmt.Errorf("FROST engine did not return local keyshare")
	}

	poolPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	if err != nil {
		return common.EmptyPubKeySet, nil, err
	}
	if err := kg.localState.SaveLocalState(storage.KeygenLocalState{
		PubKey:          poolPubKey.String(),
		LocalData:       localShare,
		ParticipantKeys: participants,
		LocalPartyKey:   localParty.String(),
		SigningEngine:   storage.SigningEngineFrost,
	}); err != nil {
		return common.EmptyPubKeySet, nil, fmt.Errorf("save FROST local state: %w", err)
	}

	kg.logger.Info().
		Int64("height", keygenBlockHeight).
		Str("pubkey", poolPubKey.String()).
		Int("members", len(participants)).
		Strs("chains", chains.Strings()).
		Msg("FROST keygen complete")
	return common.NewPubKeySet(poolPubKey, common.EmptyPubKey), nil, nil
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
