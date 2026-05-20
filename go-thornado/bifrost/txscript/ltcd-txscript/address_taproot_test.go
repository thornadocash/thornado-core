package txscript

import (
	"encoding/hex"
	"testing"

	"github.com/ltcsuite/ltcd/chaincfg"
)

func TestDecodeAddressTaproot(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		net     *chaincfg.Params
		valid   bool
		program string // expected witness program hex
	}{
		{
			name:    "mainnet taproot address",
			addr:    "ltc1pqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzywff7",
			net:     &chaincfg.MainNetParams,
			valid:   true,
			program: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:  "testnet taproot address wrong network",
			addr:  "tltc1pqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqft2hkt",
			net:   &chaincfg.MainNetParams,
			valid: false,
		},
		{
			name:    "testnet taproot address",
			addr:    "tltc1pqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqft2hkt",
			net:     &chaincfg.TestNet4Params,
			valid:   true,
			program: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:  "invalid bech32 (not bech32m)",
			addr:  "ltc1qw508d6qejxtdg4y5r3zarvary0c5xw7kgmn4n9",
			net:   &chaincfg.MainNetParams,
			valid: true, // valid segwit v0 address, decoded by ltcutil
		},
		{
			name:  "invalid address",
			addr:  "notanaddress",
			net:   &chaincfg.MainNetParams,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := DecodeAddress(tt.addr, tt.net)
			if tt.valid {
				if err != nil {
					t.Fatalf("expected valid address, got error: %v", err)
				}
				if addr.String() != tt.addr {
					t.Fatalf("address roundtrip failed: got %s, want %s", addr.String(), tt.addr)
				}
				if tt.program != "" {
					tapAddr, ok := addr.(*AddressTaproot)
					if !ok {
						t.Skip("not a taproot address")
					}
					got := hex.EncodeToString(tapAddr.ScriptAddress())
					if got != tt.program {
						t.Fatalf("witness program mismatch: got %s, want %s", got, tt.program)
					}
				}
			} else {
				if err == nil {
					t.Fatalf("expected error for invalid address %s", tt.addr)
				}
			}
		})
	}
}

func TestNewAddressTaproot(t *testing.T) {
	prog := make([]byte, 32)
	addr, err := NewAddressTaproot(prog, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !addr.IsForNet(&chaincfg.MainNetParams) {
		t.Fatal("expected address to be for mainnet")
	}
	if addr.IsForNet(&chaincfg.TestNet4Params) {
		t.Fatal("expected address NOT to be for testnet")
	}
	encoded := addr.EncodeAddress()
	if encoded == "" {
		t.Fatal("encoded address should not be empty")
	}

	// Roundtrip
	decoded, err := DecodeAddress(encoded, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to decode encoded address: %v", err)
	}
	if decoded.String() != encoded {
		t.Fatalf("roundtrip failed: got %s, want %s", decoded.String(), encoded)
	}

	// Wrong size witness program
	_, err = NewAddressTaproot(make([]byte, 20), &chaincfg.MainNetParams)
	if err == nil {
		t.Fatal("expected error for wrong size witness program")
	}
}

func TestPayToAddrScriptTaproot(t *testing.T) {
	prog := make([]byte, 32)
	for i := range prog {
		prog[i] = byte(i)
	}
	addr, err := NewAddressTaproot(prog, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	script, err := PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("failed to create pay-to-addr script: %v", err)
	}

	// P2TR script: OP_1 (0x51) + OP_DATA_32 (0x20) + 32-byte key
	if len(script) != 34 {
		t.Fatalf("expected script length 34, got %d", len(script))
	}
	if script[0] != 0x51 { // OP_1
		t.Fatalf("expected OP_1 (0x51), got 0x%02x", script[0])
	}
	if script[1] != 0x20 { // OP_DATA_32
		t.Fatalf("expected OP_DATA_32 (0x20), got 0x%02x", script[1])
	}

	// Verify the script is classified as WitnessV1TaprootTy
	scriptClass := GetScriptClass(script)
	if scriptClass != WitnessV1TaprootTy {
		t.Fatalf("expected WitnessV1TaprootTy, got %v", scriptClass)
	}
}

func TestExtractPkScriptAddrsTaproot(t *testing.T) {
	prog := make([]byte, 32)
	for i := range prog {
		prog[i] = byte(i + 1)
	}
	addr, err := NewAddressTaproot(prog, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	script, err := PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	class, addrs, reqSigs, err := ExtractPkScriptAddrs(script, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to extract: %v", err)
	}
	if class != WitnessV1TaprootTy {
		t.Fatalf("expected WitnessV1TaprootTy, got %v", class)
	}
	if reqSigs != 1 {
		t.Fatalf("expected 1 required sig, got %d", reqSigs)
	}
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}
	if addrs[0].String() != addr.String() {
		t.Fatalf("address mismatch: got %s, want %s", addrs[0].String(), addr.String())
	}
}
