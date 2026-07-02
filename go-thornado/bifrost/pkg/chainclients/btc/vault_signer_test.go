package btc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/libp2p/go-libp2p/core/peer"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	p2pstorage "github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/common"
	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"
)

type testSignCoordinator struct {
	shares map[string][]byte
}

func (c *testSignCoordinator) RunKeygen(
	ctx context.Context,
	height int64,
	participants []string,
	localParty string,
	minSigners uint16,
) (localShare []byte, pubKeyCompressed []byte, err error) {
	_ = ctx
	_ = height
	in := &frost.InProcessSessionCoordinator{}
	return in.RunKeygen(ctx, height, participants, localParty, minSigners)
}

func (c *testSignCoordinator) RunSign(
	ctx context.Context,
	_ string,
	_ int64,
	participants []string,
	localParty string,
	_ []byte,
	msg []byte,
	taprootKeyPath bool,
	scriptRoot []byte,
	childTweak []byte,
	_ string,
) ([]byte, error) {
	_ = ctx
	return frost.RunInProcessSign(participants, c.shares, localParty, msg, taprootKeyPath, scriptRoot, childTweak)
}

func TestFrostVaultSignerRemoteSignSuccess(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	allShares, err := frost.RunInProcessKeygenAll(participants, 2)
	require.NoError(t, err)
	decoded, err := frostsessions.DecodeKeyshare(allShares["node-a"])
	require.NoError(t, err)
	pubKeyBytes, err := hex.DecodeString(decoded.PublicKeyCompressed)
	require.NoError(t, err)

	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	require.NoError(t, err)

	state := &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{
		vaultPubKey.String(): {
			PubKey:          vaultPubKey.String(),
			LocalData:       allShares["node-a"],
			ParticipantKeys: participants,
			LocalPartyKey:   "node-a",
			SigningEngine:   p2pstorage.SigningEngineFrost,
		},
	}}
	signer := &frostVaultSigner{
		localState:  state,
		log:         zerolog.Nop(),
		coordinator: &testSignCoordinator{shares: allShares},
		bridge:      &stubKeysignBridge{party: participants[:2]},
		localParty:  "node-a",
	}

	digest := sha256.Sum256([]byte("payload"))
	signature, _, err := signer.RemoteSign(digest[:], common.SigningAlgoSecp256k1, vaultPubKey.String())
	require.NoError(t, err)

	require.NoError(t, verifyTaprootSignature(vaultPubKey, common.MainVaultPathIndex, digest[:], signature))
}

func TestFrostVaultSignerRemoteSignMissingState(t *testing.T) {
	signer := &frostVaultSigner{
		localState:  &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{}},
		log:         zerolog.Nop(),
		coordinator: &testSignCoordinator{},
		bridge:      &stubKeysignBridge{},
		localParty:  "node-a",
	}

	_, _, err := signer.RemoteSign(make([]byte, 32), common.SigningAlgoSecp256k1, "missing")
	require.Error(t, err)
	var keysignErr frost.KeysignError
	require.ErrorAs(t, err, &keysignErr)
}

func TestFrostVaultSignerRemoteSignWithPathSuccess(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	allShares, err := frost.RunInProcessKeygenAll(participants, 2)
	require.NoError(t, err)
	decoded, err := frostsessions.DecodeKeyshare(allShares["node-a"])
	require.NoError(t, err)
	pubKeyBytes, err := hex.DecodeString(decoded.PublicKeyCompressed)
	require.NoError(t, err)

	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	require.NoError(t, err)

	state := &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{
		vaultPubKey.String(): {
			PubKey:          vaultPubKey.String(),
			LocalData:       allShares["node-a"],
			ParticipantKeys: participants,
			LocalPartyKey:   "node-a",
			SigningEngine:   p2pstorage.SigningEngineFrost,
		},
	}}
	signer := &frostVaultSigner{
		localState:  state,
		log:         zerolog.Nop(),
		coordinator: &testSignCoordinator{shares: allShares},
		bridge:      &stubKeysignBridge{party: participants[:2]},
		localParty:  "node-a",
	}

	digest := sha256.Sum256([]byte("deposit-sweep"))
	pathIndex, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	signature, _, err := signer.RemoteSignWithPath(digest[:], common.SigningAlgoSecp256k1, vaultPubKey.String(), pathIndex)
	require.NoError(t, err)
	require.NoError(t, verifyTaprootSignature(vaultPubKey, pathIndex, digest[:], signature))
}

type stubKeysignBridge struct {
	party []string
}

func (s *stubKeysignBridge) GetKeysignParty(_ common.PubKey) (common.PubKeys, error) {
	keys := make(common.PubKeys, len(s.party))
	for i, item := range s.party {
		keys[i] = common.PubKey(item)
	}
	return keys, nil
}

func (s *stubKeysignBridge) GetBlockHeight() (int64, error) {
	return 1, nil
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

func (s *memoryLocalState) SaveAddressBook(map[peer.ID][]maddr.Multiaddr) error {
	return nil
}

func (s *memoryLocalState) RetrieveP2PAddresses() ([]maddr.Multiaddr, error) {
	return nil, nil
}