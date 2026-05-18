package keymanager

import (
	"crypto/sha256"

	addresscodec "github.com/Peersyst/xrpl-go/address-codec"
	"golang.org/x/crypto/ripemd160" //nolint:gosec,staticcheck
)

// Key derivation constants.
var (
	accountPrefix          = []byte{0x00}
	accountPublicKeyPrefix = []byte{0x23}
	SeedPrefixEd25519      = []byte{0x01, 0xe1, 0x4b} //nolint:stylecheck
)

// checksum: first four bytes of sha256^2.
func checksum(input []byte) (cksum [4]byte) {
	h := sha256.Sum256(input)
	h2 := sha256.Sum256(h[:])
	copy(cksum[:], h2[:4])
	return cksum
}

func Encode(b, prefix []byte) string {
	buf := make([]byte, 0, len(b)+len(prefix))
	buf = append(buf, prefix...)
	buf = append(buf, b...)
	cs := checksum(buf)
	buf = append(buf, cs[:]...)
	return addresscodec.EncodeBase58(buf)
}

func EncodePublicKey(pk []byte) string {
	return Encode(pk, accountPublicKeyPrefix)
}

func MasterPubKeyToAccountID(compressedMasterPubKey []byte) string {
	// Generate SHA-256 hash of public key.
	sha256Hash := sha256.Sum256(compressedMasterPubKey)

	// Generate RIPEMD160 hash.
	ripemd160Hash := ripemd160.New() //nolint:gosec
	ripemd160Hash.Write(sha256Hash[:])
	accountIDHash := ripemd160Hash.Sum(nil)

	// Add version prefix (0x00).
	accountID := make([]byte, len(accountPrefix))
	copy(accountID, accountPrefix)
	accountID = append(accountID, accountIDHash...)

	// Generate checksum (first 4 bytes of double SHA256).
	firstHash := sha256.Sum256(accountID)
	secondHash := sha256.Sum256(firstHash[:])
	checksum := secondHash[:4]

	// Combine everything.
	accountID = append(accountID, checksum...)
	return addresscodec.EncodeBase58(accountID)
}
