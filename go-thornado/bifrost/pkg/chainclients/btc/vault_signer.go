package btc

import (
	"fmt"

	btcschnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"

	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"

	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/tss"
	"github.com/thornadocash/go-thornado/common"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type frostVaultSigner struct {
	localState storage.LocalStateManager
	log        zerolog.Logger
}

func newVaultSigner(_ thornadoclient.ThornadoBridge, localState storage.LocalStateManager, log zerolog.Logger) (tss.ThornadoKeyManager, error) {
	if localState == nil {
		return nil, fmt.Errorf("FROST local state manager is required")
	}
	return &frostVaultSigner{
		localState: localState,
		log:        log,
	}, nil
}

func (s *frostVaultSigner) RemoteSign(msg []byte, algo common.SigningAlgo, vaultPubKey string) ([]byte, []byte, error) {
	return s.RemoteSignWithPath(msg, algo, vaultPubKey, common.MainVaultPathIndex)
}

func (s *frostVaultSigner) RemoteSignWithPath(msg []byte, algo common.SigningAlgo, vaultPubKey string, pathIndex uint64) ([]byte, []byte, error) {
	if algo != common.SigningAlgoSecp256k1 {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("FROST signer only supports secp256k1, got %s", algo),
		})
	}
	state, err := s.localState.GetLocalState(vaultPubKey)
	if err != nil {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}
	if state.Engine() != storage.SigningEngineFrost {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("vault %s is not a FROST keyshare", vaultPubKey),
		})
	}
	if len(state.LocalData) == 0 {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{
			FailReason: fmt.Sprintf("vault %s has empty FROST keyshare data", vaultPubKey),
		})
	}

	pubKey, err := common.NewPubKey(vaultPubKey)
	if err != nil {
		return nil, nil, err
	}
	tweakRoot, err := common.VaultPathTweakRoot(pubKey, pathIndex)
	if err != nil {
		return nil, nil, err
	}
	signature, err := frostsessions.SignTaprootTweak(state.LocalData, msg, tweakRoot)
	if err != nil {
		return nil, nil, tss.NewKeysignError(ttypes.Blame{FailReason: err.Error()})
	}
	if pathIndex == common.MainVaultPathIndex {
		secpPubKey, err := pubKey.Secp256K1()
		if err != nil {
			return nil, nil, err
		}
		if err := frostsessions.Verify(secpPubKey.SerializeCompressed(), msg, signature); err != nil {
			return nil, nil, err
		}
	} else if err := verifyTaprootSignature(pubKey, pathIndex, msg, signature); err != nil {
		return nil, nil, err
	}

	s.log.Debug().Str("vault_pub_key", vaultPubKey).Msg("created FROST vault signature")
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

func (s *frostVaultSigner) ExportAsKeyStore(password string) (*tss.EncryptedKeyJSON, error) {
	return nil, fmt.Errorf("FROST vault signer does not expose keystore export")
}

func (s *frostVaultSigner) Start() {}
func (s *frostVaultSigner) Stop()  {}
