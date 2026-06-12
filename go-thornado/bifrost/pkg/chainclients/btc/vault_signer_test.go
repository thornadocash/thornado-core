package btc

import (
	"crypto/sha256"
	"fmt"
	"testing"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/addr"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"

	p2pstorage "github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/common"
)

func TestFrostVaultSignerRemoteSignSuccess(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	shares, pubKeyBytes, err := frostsessions.KeygenWithThreshold(participants, 2)
	require.NoError(t, err)

	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	require.NoError(t, err)

	state := &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{
		vaultPubKey.String(): {
			PubKey:          vaultPubKey.String(),
			LocalData:       shares["node-a"],
			ParticipantKeys: participants,
			LocalPartyKey:   "node-a",
			SigningEngine:   p2pstorage.SigningEngineFrost,
		},
	}}
	signer := &frostVaultSigner{
		localState: state,
		log:        zerolog.Nop(),
	}

	digest := sha256.Sum256([]byte("payload"))
	signature, _, err := signer.RemoteSign(digest[:], common.SigningAlgoSecp256k1, vaultPubKey.String())
	require.NoError(t, err)

	require.NoError(t, verifyTaprootSignature(vaultPubKey, common.MainVaultPathIndex, digest[:], signature))
}

func TestFrostVaultSignerRemoteSignMissingState(t *testing.T) {
	signer := &frostVaultSigner{
		localState: &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{}},
		log:        zerolog.Nop(),
	}

	_, _, err := signer.RemoteSign(make([]byte, 32), common.SigningAlgoSecp256k1, "missing")
	require.Error(t, err)
	var keysignErr frost.KeysignError
	require.ErrorAs(t, err, &keysignErr)
}

func TestFrostVaultSignerRemoteSignWithPathSuccess(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	shares, pubKeyBytes, err := frostsessions.KeygenWithThreshold(participants, 2)
	require.NoError(t, err)

	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	require.NoError(t, err)

	state := &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{
		vaultPubKey.String(): {
			PubKey:          vaultPubKey.String(),
			LocalData:       shares["node-a"],
			ParticipantKeys: participants,
			LocalPartyKey:   "node-a",
			SigningEngine:   p2pstorage.SigningEngineFrost,
		},
	}}
	signer := &frostVaultSigner{
		localState: state,
		log:        zerolog.Nop(),
	}

	digest := sha256.Sum256([]byte("deposit-sweep"))
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	signature, _, err := signer.RemoteSignWithPath(digest[:], common.SigningAlgoSecp256k1, vaultPubKey.String(), pathIndex)
	require.NoError(t, err)
	require.NoError(t, verifyTaprootSignature(vaultPubKey, pathIndex, digest[:], signature))
}

type memoryLocalState struct {
	states map[string]p2pstorage.KeygenLocalState
}

func (s *memoryLocalState) SaveLocalState(state p2pstorage.KeygenLocalState) error {
	s.states[state.PubKey] = state
	return nil
}

func (s *memoryLocalState) GetLocalState(pubKey string) (p2pstorage.KeygenLocalState, error) {
	state, ok := s.states[pubKey]
	if !ok {
		return p2pstorage.KeygenLocalState{}, fmt.Errorf("missing local state")
	}
	return state, nil
}

func (s *memoryLocalState) SaveAddressBook(map[peer.ID]addr.AddrList) error {
	return nil
}

func (s *memoryLocalState) RetrieveP2PAddresses() (addr.AddrList, error) {
	return nil, nil
}
