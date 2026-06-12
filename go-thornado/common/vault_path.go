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
	DepositAddressLookahead uint64 = 4096
)

type VaultDepositPathType string

const (
	VaultDepositPathUser VaultDepositPathType = "user"
	VaultDepositPathNode VaultDepositPathType = "node"

	DepositPathCommitmentRoot uint64 = 0
	depositPathSegmentMask    uint64 = 0xffffffff
)

var vaultPathDomain = []byte("thornado:frost-bip86-child:v1")

func VaultDepositPathIndex(pathType VaultDepositPathType, depositIndex, commitmentIndex uint64) (uint64, error) {
	if depositIndex > depositPathSegmentMask {
		return 0, fmt.Errorf("deposit path index out of range")
	}
	if commitmentIndex != DepositPathCommitmentRoot {
		return 0, fmt.Errorf("vault deposit addresses only support root commitment path")
	}
	switch pathType {
	case VaultDepositPathUser, VaultDepositPathNode:
	default:
		return 0, fmt.Errorf("unknown vault deposit path type: %s", pathType)
	}
	return depositIndex + 1, nil
}

func VaultDepositPath(pathType VaultDepositPathType, depositIndex, commitmentIndex uint64) string {
	pathIndex, err := VaultDepositPathIndex(pathType, depositIndex, commitmentIndex)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("m/86'/0'/0'/0/%d", pathIndex)
}

func UserSecretPath(pathType VaultDepositPathType, depositIndex, commitmentIndex uint64) string {
	return fmt.Sprintf("tc84/btc/%s/%d'/%d'", pathType, depositIndex, commitmentIndex)
}

func VaultDepositLookaheadPathIndexes(pathType VaultDepositPathType) ([]uint64, error) {
	indexes := make([]uint64, 0, DepositAddressLookahead)
	for n := uint64(0); n < DepositAddressLookahead; n++ {
		pathIndex, err := VaultDepositPathIndex(pathType, n, DepositPathCommitmentRoot)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, pathIndex)
	}
	return indexes, nil
}

func VaultPathTweakRoot(pubkey PubKey, pathIndex uint64) ([]byte, error) {
	if pathIndex == MainVaultPathIndex {
		return nil, nil
	}
	basePubKey, err := DeriveBTCBIP86InternalPubKey(pubkey, MainVaultPathIndex)
	if err != nil {
		return nil, err
	}
	return vaultChildTweak(basePubKey, pathIndex), nil
}

func DeriveBTCBIP86InternalPubKey(pubkey PubKey, pathIndex uint64) (*btcec.PublicKey, error) {
	secpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return nil, err
	}
	basePubKey, err := btcec.ParsePubKey(secpPubKey.SerializeCompressed())
	if err != nil {
		return nil, err
	}
	if pathIndex == MainVaultPathIndex {
		return basePubKey, nil
	}
	childTweak := vaultChildTweak(basePubKey, pathIndex)
	return addTweak(basePubKey, childTweak), nil
}

func DeriveBTCTaprootPubKey(pubkey PubKey, pathIndex uint64) ([]byte, error) {
	internalPubKey, err := DeriveBTCBIP86InternalPubKey(pubkey, pathIndex)
	if err != nil {
		return nil, err
	}
	outputKey := taprootOutputKey(internalPubKey, nil)
	return btcschnorr.SerializePubKey(outputKey), nil
}

func vaultChildTweak(pubKey *btcec.PublicKey, pathIndex uint64) []byte {
	var index [8]byte
	binary.BigEndian.PutUint64(index[:], pathIndex)
	h := sha256.New()
	h.Write(vaultPathDomain)
	h.Write(pubKey.SerializeCompressed())
	h.Write(index[:])
	return h.Sum(nil)
}

func addTweak(pubKey *btcec.PublicKey, tweak []byte) *btcec.PublicKey {
	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(tweak))

	var pubPoint btcec.JacobianPoint
	pubKey.AsJacobian(&pubPoint)

	var tweakPoint, tweakedPoint btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&tweakScalar, &tweakPoint)
	btcec.AddNonConst(&pubPoint, &tweakPoint, &tweakedPoint)
	tweakedPoint.ToAffine()

	return btcec.NewPublicKey(&tweakedPoint.X, &tweakedPoint.Y)
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
