package common

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	btcschnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil"
)

const (
	MainVaultPathIndex      uint64 = 0
	FirstDepositPathIndex   uint64 = 1
	DepositAddressLookahead uint64 = 4096
)

var vaultPathDomain = []byte("thornado:vault-path:v1")

func VaultPathTweakRoot(pubkey PubKey, pathIndex uint64) ([]byte, error) {
	if pathIndex == MainVaultPathIndex {
		return nil, nil
	}
	secpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return nil, err
	}
	var index [8]byte
	binary.BigEndian.PutUint64(index[:], pathIndex)
	h := sha256.New()
	h.Write(vaultPathDomain)
	h.Write(secpPubKey.SerializeCompressed())
	h.Write(index[:])
	return h.Sum(nil), nil
}

func DeriveBTCTaprootPubKey(pubkey PubKey, pathIndex uint64) ([]byte, error) {
	secpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return nil, err
	}
	basePubKey, err := btcec.ParsePubKey(secpPubKey.SerializeCompressed())
	if err != nil {
		return nil, err
	}
	if pathIndex == MainVaultPathIndex {
		return btcschnorr.SerializePubKey(basePubKey), nil
	}
	tweakRoot, err := VaultPathTweakRoot(pubkey, pathIndex)
	if err != nil {
		return nil, err
	}
	childPubKey := taprootOutputKey(basePubKey, tweakRoot)
	return btcschnorr.SerializePubKey(childPubKey), nil
}

func taprootOutputKey(pubKey *btcec.PublicKey, scriptRoot []byte) *btcec.PublicKey {
	internalKey, _ := btcschnorr.ParsePubKey(btcschnorr.SerializePubKey(pubKey))
	tapTweakHash := chainhash.TaggedHash(chainhash.TagTapTweak, btcschnorr.SerializePubKey(internalKey), scriptRoot)

	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(tapTweakHash))

	var internalPoint btcec.JacobianPoint
	internalKey.AsJacobian(&internalPoint)

	var tPoint, taprootKey btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&tweakScalar, &tPoint)
	btcec.AddNonConst(&internalPoint, &tPoint, &taprootKey)
	taprootKey.ToAffine()

	return btcec.NewPublicKey(&taprootKey.X, &taprootKey.Y)
}

func DeriveBTCTaprootAddress(pubkey PubKey, pathIndex uint64) (Address, error) {
	pubKeyBytes, err := DeriveBTCTaprootPubKey(pubkey, pathIndex)
	if err != nil {
		return NoAddress, err
	}
	net, err := BTCChainParams()
	if err != nil {
		return NoAddress, err
	}
	addr, err := btcutil.NewAddressTaproot(pubKeyBytes, net)
	if err != nil {
		return NoAddress, fmt.Errorf("fail to derive taproot address: %w", err)
	}
	return NewAddress(addr.String())
}

func BTCChainParams() (*chaincfg.Params, error) {
	switch CurrentChainNetwork {
	case MockNet:
		return &chaincfg.RegressionNetParams, nil
	case TestNet:
		return &chaincfg.TestNet3Params, nil
	case MainNet, StageNet, ChainNet:
		return &chaincfg.MainNetParams, nil
	default:
		return nil, fmt.Errorf("unsupported bitcoin network: %v", CurrentChainNetwork)
	}
}
