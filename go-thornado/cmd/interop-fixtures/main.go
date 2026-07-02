// Command interop-fixtures emits authoritative wire-format bytes from the real
// Go bifrost p2p types, for the Rust bifrost-signer crate to test against.
//
// Run: go run ./cmd/interop-fixtures <output-dir>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

func main() {
	outDir := "."
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	// Fixture A: a FROST keysign WrappedMessage. Payload is the raw
	// ProtocolMessage JSON bytes exactly as the Rust FFI emits them; Go's
	// json.Marshal base64-encodes the []byte Payload field.
	protocolMsg := []byte(`{"kind":"sign_round1","from":"party0","to":["party1","party2"],"payload":"AAECAwQF"}`)
	wrapped := messages.WrappedMessage{
		MessageType: messages.FROSTFrostKeySignMsg,
		MsgID:       "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
		Payload:     protocolMsg,
	}
	wrappedBytes, err := json.Marshal(wrapped)
	if err != nil {
		panic(err)
	}
	write(outDir, "wrapped_keysign.json", wrappedBytes)

	// Fixture B: a JoinPartyLeaderComm success response (gogo-proto wire bytes).
	comm := messages.JoinPartyLeaderComm{
		ID:      "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
		MsgType: "response",
		Type:    messages.JoinPartyLeaderComm_Success,
		PeerIDs: []string{"party0", "party1", "party2"},
	}
	commBytes, err := comm.Marshal()
	if err != nil {
		panic(err)
	}
	write(outDir, "join_party_response.pb", commBytes)

	// Fixture C: a JoinPartyLeaderComm request (minimal).
	req := messages.JoinPartyLeaderComm{
		ID:      "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
		MsgType: "request",
	}
	reqBytes, err := req.Marshal()
	if err != nil {
		panic(err)
	}
	write(outDir, "join_party_request.pb", reqBytes)

	fmt.Printf("wrote fixtures to %s\n", outDir)
}

func write(dir, name string, data []byte) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("  %s (%d bytes)\n", name, len(data))
}
