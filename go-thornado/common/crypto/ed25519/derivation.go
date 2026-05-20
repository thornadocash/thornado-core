package ed25519

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/tyler-smith/go-bip39"
)

const (
	// HardenedKeyStart is the index where hardened keys start
	// In BIP32/SLIP-0010, indices >= 0x80000000 are hardened
	HardenedKeyStart uint32 = 0x80000000

	// Ed25519Curve is the curve name used for Ed25519 SLIP-0010 derivation
	// Unlike Bitcoin which uses "Bitcoin seed", Ed25519 SLIP-0010 uses "ed25519 seed"
	// This is the string used as the HMAC key when deriving the master key
	Ed25519Curve = "ed25519 seed"

	// Ed25519HDPath is the default HD path for Ed25519 keys
	HDPath = `m/44'/931'/0'/0'/0'`
)

// SignerNameEDDSA returns the signer name for EdDSA keys.
// Store as neighbor to secp256k1 key for simplicity.
func SignerNameEDDSA(signerName string) string {
	return fmt.Sprintf("ed-%s", signerName)
}

// DerivationPath represents a BIP-32 derivation path
type DerivationPath []uint32

// ParseDerivationPath parses a string derivation path into a slice of uint32 indices
func ParseDerivationPath(path string) (DerivationPath, error) {
	if !strings.HasPrefix(path, "m/") {
		return nil, fmt.Errorf("invalid derivation path: %s, should start with 'm/'", path)
	}

	path = path[2:]
	segments := strings.Split(path, "/")
	derivationPath := make(DerivationPath, 0, len(segments))

	for _, segment := range segments {
		var value uint32

		if strings.HasSuffix(segment, "'") || strings.HasSuffix(segment, "h") {
			// Hardened key
			valueStr := segment[:len(segment)-1]
			val, err := strconv.ParseUint(valueStr, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid hardened segment: %s in path: %s", segment, path)
			}
			value = HardenedKeyStart + uint32(val)
		} else {
			// Non-hardened key - Note: For Ed25519, only hardened keys work per SLIP-0010
			val, err := strconv.ParseUint(segment, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid segment: %s in path: %s", segment, path)
			}
			value = uint32(val)
		}

		derivationPath = append(derivationPath, value)
	}

	return derivationPath, nil
}

// SeedFromMnemonic generates a seed from a BIP39 mnemonic with an optional passphrase
func SeedFromMnemonic(mnemonic, passphrase string) ([]byte, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	return bip39.NewSeed(mnemonic, passphrase), nil
}

// MasterKeyFromSeed generates a master key and chain code from a seed following SLIP-0010
func MasterKeyFromSeed(seed []byte) ([]byte, []byte, error) {
	// Create HMAC-SHA512 with key "ed25519 seed"
	h := hmac.New(sha512.New, []byte(Ed25519Curve))
	_, err := h.Write(seed)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating master key: %v", err)
	}

	// Get key and chain code from hash
	I := h.Sum(nil)
	privateKey := I[:32]
	chainCode := I[32:]

	// In SLIP-0010, for Ed25519, we don't need to check for an invalid key
	// as any 32-byte sequence is valid

	return privateKey, chainCode, nil
}

// DerivePrivateKey derives a child key from a parent key and chain code following SLIP-0010
func DerivePrivateKey(path DerivationPath, masterKey, chainCode []byte) ([]byte, []byte, error) {
	key := masterKey
	code := chainCode

	for _, index := range path {
		// In SLIP-0010 for Ed25519, only hardened keys are supported
		if index < HardenedKeyStart {
			return nil, nil, fmt.Errorf("non-hardened key derivation is not supported for Ed25519")
		}

		// Data to HMAC: 0x00 || parent private key || index (BE)
		data := make([]byte, 1+32+4)
		data[0] = 0x00
		copy(data[1:33], key)
		binary.BigEndian.PutUint32(data[33:], index)

		h := hmac.New(sha512.New, code)
		_, err := h.Write(data)
		if err != nil {
			return nil, nil, fmt.Errorf("error generating child key: %v", err)
		}

		// Get key and chain code from hash
		I := h.Sum(nil)
		key = I[:32]
		code = I[32:]
	}

	return key, code, nil
}

// DeriveKeypairFromMnemonic derives a Solana keypair from a mnemonic and derivation path
func DeriveKeypairFromMnemonic(mnemonic, passphrase, path string) ([]byte, error) {
	derivationPath, err := ParseDerivationPath(path)
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path: %v", err)
	}

	seed, err := SeedFromMnemonic(mnemonic, passphrase)
	if err != nil {
		return nil, fmt.Errorf("invalid mnemonic: %v", err)
	}

	masterKey, chainCode, err := MasterKeyFromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("error generating master key: %v", err)
	}

	derivedKey, _, err := DerivePrivateKey(derivationPath, masterKey, chainCode)
	if err != nil {
		return nil, fmt.Errorf("error deriving private key: %v", err)
	}

	return ed25519.NewKeyFromSeed(derivedKey), nil
}

// GetLegacyKeypairFromMnemonic gets a Solana keypair using the legacy method
// (using the first 32 bytes of the seed directly as private key without path derivation)
func GetLegacyKeypairFromMnemonic(mnemonic, passphrase string) ([]byte, error) {
	seed, err := SeedFromMnemonic(mnemonic, passphrase)
	if err != nil {
		return nil, fmt.Errorf("invalid mnemonic: %v", err)
	}

	// Legacy method: take the first 32 bytes of the seed
	derivedKey := seed[:32]

	return ed25519.NewKeyFromSeed(derivedKey), nil
}

func GetPrivateKeyFromMnemonic(mnemonic, passphrase, path string) ([]byte, error) {
	if path == "" {
		return GetLegacyKeypairFromMnemonic(mnemonic, passphrase)
	}

	return DeriveKeypairFromMnemonic(mnemonic, passphrase, path)
}
