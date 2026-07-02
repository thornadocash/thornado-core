package types

import "testing"

func TestShielderNoteRecordLeafIndexRoundTrip(t *testing.T) {
	original := ShielderNoteRecord{
		Commitment:       "0xdeadbeef",
		DenominationSats: 100_000_000,
		CreatedHeight:    42,
		LeafIndex:        7,
	}
	encoded, err := original.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ShielderNoteRecord
	if err := decoded.Unmarshal(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.LeafIndex != original.LeafIndex {
		t.Fatalf("leaf index: got %d want %d", decoded.LeafIndex, original.LeafIndex)
	}
	if decoded.Commitment != original.Commitment ||
		decoded.DenominationSats != original.DenominationSats ||
		decoded.CreatedHeight != original.CreatedHeight {
		t.Fatalf("other fields corrupted: %#v", decoded)
	}
	if decoded.Size() != len(encoded) {
		t.Fatalf("size mismatch: Size()=%d encoded=%d", decoded.Size(), len(encoded))
	}

	// Zero leaf index must not be emitted (proto3 default), preserving wire compat
	// with records written before the field existed.
	legacy := ShielderNoteRecord{Commitment: "c", DenominationSats: 1, CreatedHeight: 2}
	legacyEncoded, err := legacy.Marshal()
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	var legacyDecoded ShielderNoteRecord
	if err := legacyDecoded.Unmarshal(legacyEncoded); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if legacyDecoded.LeafIndex != 0 {
		t.Fatalf("expected zero leaf index, got %d", legacyDecoded.LeafIndex)
	}
}
