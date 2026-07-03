package common

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

// Conformance vectors for vault taproot key/script derivation.
//
// Reference of truth: rust-bitcoin (BIP341/BIP86) in the Rust bifrost-signer
// crate. These vectors capture the Go DeriveBTC* output; the Rust side derives
// from the same compressed vault pubkey + path index and must match the output
// x-only key and scriptPubKey byte-for-byte.
//
// Regenerate with:
//   BIFROST_WRITE_FIXTURES=1 go test -run TestVaultDerivationGolden ./common/

type vaultDerivationVector struct {
	CompressedPubKeyHex string `json:"compressed_pubkey_hex"`
	PathIndex           uint64 `json:"path_index"`
	InternalXOnlyHex    string `json:"internal_xonly_hex"`
	OutputXOnlyHex      string `json:"output_xonly_hex"`
	ScriptHex           string `json:"script_hex"`
}

func fixedVaultPubKey(t *testing.T) (PubKey, string) {
	t.Helper()
	// Deterministic private key → compressed secp256k1 pubkey (the Rust input).
	var secret [32]byte
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	priv, _ := btcec.PrivKeyFromBytes(secret[:])
	compressed := priv.PubKey().SerializeCompressed()

	cosmosPk := &secp256k1.PubKey{Key: compressed}
	spk, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, cosmosPk)
	if err != nil {
		t.Fatalf("bech32ify: %v", err)
	}
	pk, err := NewPubKey(spk)
	if err != nil {
		t.Fatalf("new pubkey: %v", err)
	}
	return pk, hex.EncodeToString(compressed)
}

func p2trScriptFromXOnly(xonly []byte) []byte {
	s := make([]byte, 0, 34)
	s = append(s, 0x51, 0x20)
	return append(s, xonly...)
}

func TestVaultDerivationGolden(t *testing.T) {
	pk, compHex := fixedVaultPubKey(t)

	paths := []uint64{MainVaultPathIndex, 1, 5, 42}
	vectors := make([]vaultDerivationVector, 0, len(paths))
	for _, path := range paths {
		internal, err := DeriveBTCBIP86InternalPubKey(pk, path)
		if err != nil {
			t.Fatalf("internal derive path %d: %v", path, err)
		}
		internalXOnly := schnorrSerialize(internal)

		outputXOnly, err := DeriveBTCTaprootPubKey(pk, path)
		if err != nil {
			t.Fatalf("output derive path %d: %v", path, err)
		}
		if len(outputXOnly) != 32 {
			t.Fatalf("path %d: output xonly len %d, want 32", path, len(outputXOnly))
		}

		vectors = append(vectors, vaultDerivationVector{
			CompressedPubKeyHex: compHex,
			PathIndex:           path,
			InternalXOnlyHex:    hex.EncodeToString(internalXOnly),
			OutputXOnlyHex:      hex.EncodeToString(outputXOnly),
			ScriptHex:           hex.EncodeToString(p2trScriptFromXOnly(outputXOnly)),
		})
	}

	if os.Getenv("BIFROST_WRITE_FIXTURES") == "1" {
		dir := "../../test-fixtures/interop"
		if alt := os.Getenv("BIFROST_FIXTURE_DIR"); alt != "" {
			dir = alt
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		blob, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		path := filepath.Join(dir, "vault_derivation_vectors.json")
		if err := os.WriteFile(path, blob, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Logf("wrote %d vault derivation vectors to %s", len(vectors), path)
	}
}

// schnorrSerialize returns the 32-byte x-only serialization of a pubkey,
// matching btcschnorr.SerializePubKey (drops the y-coordinate).
func schnorrSerialize(pk *btcec.PublicKey) []byte {
	return pk.SerializeCompressed()[1:]
}
