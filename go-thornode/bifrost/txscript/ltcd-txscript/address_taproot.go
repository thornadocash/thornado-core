package txscript

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcutil/bech32"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcutil"
)

// Compile-time assertion that AddressTaproot implements ltcutil.Address.
var _ ltcutil.Address = (*AddressTaproot)(nil)

// AddressTaproot is an Address for a pay-to-taproot (P2TR) output for Litecoin.
type AddressTaproot struct {
	hrp            string
	witnessVersion byte
	witnessProgram []byte
}

// NewAddressTaproot returns a new AddressTaproot.
func NewAddressTaproot(witnessProg []byte, net *chaincfg.Params) (*AddressTaproot, error) {
	return newAddressTaproot(net.Bech32HRPSegwit, witnessProg)
}

func newAddressTaproot(hrp string, witnessProg []byte) (*AddressTaproot, error) {
	if len(witnessProg) != 32 {
		return nil, fmt.Errorf("witness program must be 32 bytes for p2tr")
	}
	return &AddressTaproot{
		hrp:            strings.ToLower(hrp),
		witnessVersion: 0x01,
		witnessProgram: witnessProg,
	}, nil
}

// EncodeAddress returns the bech32m string encoding of the taproot address.
func (a *AddressTaproot) EncodeAddress() string {
	str, err := encodeSegWitAddressTaproot(a.hrp, a.witnessVersion, a.witnessProgram)
	if err != nil {
		return ""
	}
	return str
}

// ScriptAddress returns the witness program for this address.
func (a *AddressTaproot) ScriptAddress() []byte {
	return a.witnessProgram
}

// IsForNet returns whether the AddressTaproot is associated with the
// passed litecoin network.
func (a *AddressTaproot) IsForNet(net *chaincfg.Params) bool {
	return a.hrp == net.Bech32HRPSegwit
}

// String returns the bech32m encoding of the taproot address.
func (a *AddressTaproot) String() string {
	return a.EncodeAddress()
}

// encodeSegWitAddressTaproot encodes a taproot address using bech32m.
func encodeSegWitAddressTaproot(hrp string, witnessVersion byte, witnessProgram []byte) (string, error) {
	converted, err := bech32.ConvertBits(witnessProgram, 8, 5, true)
	if err != nil {
		return "", err
	}
	combined := make([]byte, len(converted)+1)
	combined[0] = witnessVersion
	copy(combined[1:], converted)
	return bech32.EncodeM(hrp, combined)
}

// DecodeAddress wraps ltcutil.DecodeAddress and adds support for taproot
// (bech32m, witness version 1) addresses.
func DecodeAddress(addr string, net *chaincfg.Params) (ltcutil.Address, error) {
	// First try the standard ltcutil decoder.
	decoded, err := ltcutil.DecodeAddress(addr, net)
	if err == nil {
		return decoded, nil
	}

	// If standard decode failed, try to decode as a taproot (bech32m) address.
	return decodeTaprootAddress(addr, net)
}

// decodeTaprootAddress attempts to decode a bech32m-encoded taproot address.
func decodeTaprootAddress(addr string, net *chaincfg.Params) (*AddressTaproot, error) {
	// Use btcutil's bech32 package which supports bech32m via DecodeGeneric.
	hrp, data, version, err := bech32.DecodeGeneric(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bech32m address: %w", err)
	}

	// Taproot addresses must use bech32m encoding.
	if version != bech32.VersionM {
		return nil, fmt.Errorf("taproot address must use bech32m encoding")
	}

	// Verify the HRP matches the expected network.
	if hrp != net.Bech32HRPSegwit {
		return nil, fmt.Errorf("invalid hrp %q (expected %q)", hrp, net.Bech32HRPSegwit)
	}

	// The first byte of the decoded data is the witness version.
	if len(data) < 1 {
		return nil, fmt.Errorf("empty witness program")
	}
	witnessVersion := data[0]
	if witnessVersion != 1 {
		return nil, fmt.Errorf("unsupported witness version %d for taproot", witnessVersion)
	}

	// Convert the remaining 5-bit data to 8-bit.
	witnessProgram, err := bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return nil, fmt.Errorf("failed to convert witness program: %w", err)
	}

	return newAddressTaproot(hrp, witnessProgram)
}
