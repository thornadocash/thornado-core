package zecutil

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/stretchr/testify/require"

	"golang.org/x/crypto/ripemd160" //nolint:gosec,staticcheck
)

// --- common.go tests ---

func TestBinaryFreeList_BorrowReturn(t *testing.T) {
	fl := make(binaryFreeList, 2)

	// Borrow from empty free list (allocates new)
	buf := fl.Borrow()
	require.Len(t, buf, 8)

	// Return to free list
	fl.Return(buf)

	// Borrow should get the same buffer back from the channel
	buf2 := fl.Borrow()
	require.Len(t, buf2, 8)

	// Fill the free list then return one more (should be discarded)
	fl.Return(make([]byte, 8))
	fl.Return(make([]byte, 8))
	fl.Return(make([]byte, 8)) // should be dropped since cap is 2
}

func TestWriteVarInt_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		val  uint64
	}{
		{"single byte", 0x01},
		{"max single byte", 0xfc},
		{"uint16 min", 0xfd},
		{"uint16 max", math.MaxUint16},
		{"uint32 min", math.MaxUint16 + 1},
		{"uint32 max", math.MaxUint32},
		{"uint64 min", math.MaxUint32 + 1},
		{"uint64 large", math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteVarInt(&buf, 0, tt.val)
			require.NoError(t, err)

			// Verify round-trip through ReadVarInt
			got, err := ReadVarInt(bytes.NewReader(buf.Bytes()), 0)
			require.NoError(t, err)
			require.Equal(t, tt.val, got)
		})
	}
}

func TestWriteVarInt_WriterError(t *testing.T) {
	err := WriteVarInt(&errWriter{}, 0, 0x01)
	require.Error(t, err)
}

func TestWriteVarBytes(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("hello")
	err := WriteVarBytes(&buf, 0, data)
	require.NoError(t, err)
	require.True(t, buf.Len() > len(data))

	// Read back
	r := bytes.NewReader(buf.Bytes())
	got, err := ReadVarBytes(r, 0, 100)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestWriteVarBytes_WriterError(t *testing.T) {
	err := WriteVarBytes(&errWriter{}, 0, []byte("hi"))
	require.Error(t, err)
}

// --- ReadVarInt / ReadVarBytes tests ---

func TestReadVarInt_NonCanonical(t *testing.T) {
	// 0xff prefix but value fits in uint32
	buf := []byte{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err := ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-canonical")

	// 0xfe prefix but value fits in uint16
	buf = []byte{0xfe, 0x01, 0x00, 0x00, 0x00}
	_, err = ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-canonical")

	// 0xfd prefix but value < 0xfd
	buf = []byte{0xfd, 0x01, 0x00}
	_, err = ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-canonical")
}

func TestReadVarInt_EmptyReader(t *testing.T) {
	_, err := ReadVarInt(bytes.NewReader(nil), 0)
	require.Error(t, err)
}

func TestReadVarInt_TruncatedData(t *testing.T) {
	// 0xff prefix but only 4 bytes of data
	buf := []byte{0xff, 0x01, 0x02, 0x03, 0x04}
	_, err := ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)

	// 0xfe prefix but only 2 bytes
	buf = []byte{0xfe, 0x01, 0x02}
	_, err = ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)

	// 0xfd prefix but only 1 byte
	buf = []byte{0xfd, 0x01}
	_, err = ReadVarInt(bytes.NewReader(buf), 0)
	require.Error(t, err)
}

func TestReadVarBytes_TooLarge(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteVarInt(&buf, 0, 100)
	_, err := ReadVarBytes(bytes.NewReader(buf.Bytes()), 0, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too large")
}

func TestReadVarBytes_TruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteVarInt(&buf, 0, 10) // says 10 bytes
	buf.Write([]byte{1, 2, 3})   // only 3 bytes
	_, err := ReadVarBytes(bytes.NewReader(buf.Bytes()), 0, 100)
	require.Error(t, err)
}

// --- msgtx.go tests ---

func TestZecTxFromHex_RoundTrip(t *testing.T) {
	mtx, err := ZecTxFromHex(rawTx)
	require.NoError(t, err)
	require.NotNil(t, mtx)
	require.Equal(t, 1, len(mtx.TxIn))
	require.Equal(t, 2, len(mtx.TxOut))

	hexStr, err := mtx.ZecToHex()
	require.NoError(t, err)
	require.Equal(t, strings.ToLower(rawTx), strings.ToLower(hexStr))
}

func TestZecTxFromHex_InvalidHex(t *testing.T) {
	_, err := ZecTxFromHex("zzzz")
	require.Error(t, err)
}

func TestZecTxFromHex_InvalidData(t *testing.T) {
	_, err := ZecTxFromHex("0102030405")
	require.Error(t, err)
}

func TestZecDecode_NotOverwintered(t *testing.T) {
	// version=3 but without overwintered flag (bit 31 clear)
	var buf bytes.Buffer
	var v [4]byte
	littleEndian.PutUint32(v[:], 3) // no overwintered flag
	buf.Write(v[:])
	mtx := &MsgTx{MsgTx: wire.NewMsgTx(3)}
	err := mtx.ZecDeserialize(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not overwintered")
}

func TestZecDecode_UnknownVersionGroupID(t *testing.T) {
	var buf bytes.Buffer
	var v [4]byte
	littleEndian.PutUint32(v[:], 3|(1<<31)) // overwintered v3
	buf.Write(v[:])
	littleEndian.PutUint32(v[:], 0xDEADBEEF) // unknown group ID
	buf.Write(v[:])
	mtx := &MsgTx{MsgTx: wire.NewMsgTx(3)}
	err := mtx.ZecDeserialize(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown versionGroupID")
}

func TestZecDecode_Sapling(t *testing.T) {
	// Build a minimal sapling tx (v4)
	var buf bytes.Buffer
	var v [4]byte

	// version with overwintered flag
	littleEndian.PutUint32(v[:], 4|(1<<31))
	buf.Write(v[:])

	// sapling group ID
	littleEndian.PutUint32(v[:], versionSaplingGroupID)
	buf.Write(v[:])

	// nIn = 0
	_ = WriteVarInt(&buf, 0, 0)

	// nOut = 0
	_ = WriteVarInt(&buf, 0, 0)

	// locktime
	littleEndian.PutUint32(v[:], 0)
	buf.Write(v[:])

	// expiry height
	littleEndian.PutUint32(v[:], 100)
	buf.Write(v[:])

	// valueBalance (8 bytes)
	var vb [8]byte
	buf.Write(vb[:])

	// nShieldedSpend = 0
	_ = WriteVarInt(&buf, 0, 0)

	// nShieldedOutput = 0
	_ = WriteVarInt(&buf, 0, 0)

	// nJoinSplits = 0
	_ = WriteVarInt(&buf, 0, 0)

	mtx := &MsgTx{MsgTx: wire.NewMsgTx(4)}
	err := mtx.ZecDeserialize(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Equal(t, versionSapling, mtx.Version)
	require.Equal(t, uint32(100), mtx.ExpiryHeight)
}

func TestZecDecode_SaplingNonTransparent(t *testing.T) {
	buf2 := buildSaplingNonTransparentTx(1, 0)
	mtx := &MsgTx{MsgTx: wire.NewMsgTx(4)}
	err := mtx.ZecDeserialize(bytes.NewReader(buf2))
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-transparent sapling")
}

func buildSaplingNonTransparentTx(nSpend, nOutput uint64) []byte {
	var buf bytes.Buffer
	var v [4]byte
	littleEndian.PutUint32(v[:], 4|(1<<31))
	buf.Write(v[:])
	littleEndian.PutUint32(v[:], versionSaplingGroupID)
	buf.Write(v[:])
	_ = WriteVarInt(&buf, 0, 0)
	_ = WriteVarInt(&buf, 0, 0)
	littleEndian.PutUint32(v[:], 0)
	buf.Write(v[:])
	littleEndian.PutUint32(v[:], 0)
	buf.Write(v[:])
	var vb [8]byte
	buf.Write(vb[:])
	_ = WriteVarInt(&buf, 0, nSpend)
	_ = WriteVarInt(&buf, 0, nOutput)
	return buf.Bytes()
}

func TestZecDecode_JoinSplitsPresent(t *testing.T) {
	// Build overwinter tx with joinsplits > 0
	var buf bytes.Buffer
	var v [4]byte
	littleEndian.PutUint32(v[:], 3|(1<<31))
	buf.Write(v[:])
	littleEndian.PutUint32(v[:], versionOverwinterGroupID)
	buf.Write(v[:])
	_ = WriteVarInt(&buf, 0, 0) // nIn
	_ = WriteVarInt(&buf, 0, 0) // nOut
	littleEndian.PutUint32(v[:], 0)
	buf.Write(v[:]) // locktime
	littleEndian.PutUint32(v[:], 0)
	buf.Write(v[:])             // expiry
	_ = WriteVarInt(&buf, 0, 1) // nJoinSplits = 1

	mtx := &MsgTx{MsgTx: wire.NewMsgTx(3)}
	err := mtx.ZecDeserialize(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "joinsplits")
}

func TestZecEncode_SaplingVersion(t *testing.T) {
	newTx := wire.NewMsgTx(4)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	var buf bytes.Buffer
	err := zecTx.ZecEncode(&buf, 0, wire.BaseEncoding)
	require.NoError(t, err)

	// Verify it round-trips
	mtx2 := &MsgTx{MsgTx: wire.NewMsgTx(4)}
	err = mtx2.ZecDeserialize(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Equal(t, versionSapling, mtx2.Version)
}

func TestZecEncode_WitnessEncoding(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	txIn.Witness = wire.TxWitness{[]byte{0x01, 0x02}, []byte{0x03, 0x04}}
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 1000}
	var buf bytes.Buffer
	err := zecTx.ZecEncode(&buf, 0, wire.WitnessEncoding)
	require.NoError(t, err)
	require.True(t, buf.Len() > 0)
}

func TestMsgTx_TxHash(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 1), []byte{0x01}, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}
	hash := zecTx.TxHash()
	require.NotEqual(t, chainhash.Hash{}, hash)
}

// --- hashcache.go tests ---

func TestNewTxSigHashes(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 1), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}
	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)
	require.NotNil(t, hashes)
	require.NotEqual(t, chainhash.Hash{}, hashes.HashPrevOuts)
	require.NotEqual(t, chainhash.Hash{}, hashes.HashSequence)
	require.NotEqual(t, chainhash.Hash{}, hashes.HashOutputs)
}

// --- paytoaddr.go tests ---

func TestPayToAddrScript_PubKeyHash(t *testing.T) {
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xab}, ripemd160.Size))
	addr := NewAddressPubKeyHash(hash, "mainnet")

	script, err := PayToAddrScript(addr)
	require.NoError(t, err)
	require.NotEmpty(t, script)
	// Should be a standard P2PKH script
	require.Equal(t, byte(txscript.OP_DUP), script[0])
}

func TestPayToAddrScript_ScriptHash(t *testing.T) {
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xcd}, ripemd160.Size))
	addr := NewAddressScriptHash(hash, "mainnet")

	script, err := PayToAddrScript(addr)
	require.NoError(t, err)
	require.NotEmpty(t, script)
	// Should be a standard P2SH script
	require.Equal(t, byte(txscript.OP_HASH160), script[0])
}

func TestPayToAddrScript_NilPubKeyHash(t *testing.T) {
	var addr *ZecAddressPubKeyHash
	_, err := PayToAddrScript(addr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil address")
}

func TestPayToAddrScript_NilScriptHash(t *testing.T) {
	var addr *ZecAddressScriptHash
	_, err := PayToAddrScript(addr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil address")
}

// --- sign.go tests ---

func TestBlake2bSignatureHash_InvalidIdx(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)

	_, err = Blake2bSignatureHash([]byte{0x76}, hashes, txscript.SigHashAll, zecTx, 5, 100000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "idx 5")
}

func TestBlake2bSignatureHash_SigHashAnyOneCanPay(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)

	hash, err := Blake2bSignatureHash([]byte{0x76}, hashes, txscript.SigHashAll|txscript.SigHashAnyOneCanPay, zecTx, 0, 100000)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestBlake2bSignatureHash_SigHashSingle(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)

	hash, err := Blake2bSignatureHash([]byte{0x76}, hashes, txscript.SigHashSingle, zecTx, 0, 100000)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestBlake2bSignatureHash_SigHashNone(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)

	hash, err := Blake2bSignatureHash([]byte{0x76}, hashes, txscript.SigHashNone, zecTx, 0, 100000)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestBlake2bSignatureHash_Sapling(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(4) // sapling
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	hashes, err := NewTxSigHashes(zecTx)
	require.NoError(t, err)

	hash, err := Blake2bSignatureHash([]byte{0x76}, hashes, txscript.SigHashAll, zecTx, 0, 100000)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestSignatureScript_Uncompressed(t *testing.T) {
	privKey, err := btcec.NewPrivateKey(btcec.S256())
	require.NoError(t, err)

	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	script, err := SignatureScript(zecTx, 0, []byte{0x76, 0xa9, 0x14}, txscript.SigHashAll, privKey, false, 200000)
	require.NoError(t, err)
	require.NotEmpty(t, script)
}

func TestMergeScripts(t *testing.T) {
	// default case: longer script wins
	sig1 := []byte{1, 2, 3, 4, 5}
	sig2 := []byte{1, 2}
	result := mergeScripts(nil, nil, 0, nil, txscript.PubKeyHashTy, nil, 0, sig1, sig2)
	require.Equal(t, sig1, result)

	// Previous script is longer
	result = mergeScripts(nil, nil, 0, nil, txscript.PubKeyHashTy, nil, 0, sig2, sig1)
	require.Equal(t, sig1, result)
}

func TestSigHashKey(t *testing.T) {
	// Test with height 0 (should use first upgrade param)
	key := sigHashKey(0)
	require.NotEmpty(t, key)
	require.True(t, strings.HasPrefix(string(key), blake2BSigHash))

	// Test with very high height
	key2 := sigHashKey(999999)
	require.NotEmpty(t, key2)
}

// --- zecaddr.go tests ---

func TestDecodeAddress_UnknownNet(t *testing.T) {
	_, err := DecodeAddress("tmF834qorixnCV18bVrkM8WN1Xasy5eXcZV", "unknown")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown net")
}

func TestDecodeAddress_InvalidBase58Length(t *testing.T) {
	_, err := DecodeAddress("abc", "testnet3")
	require.Error(t, err)
}

func TestDecodeAddress_InvalidChecksum(t *testing.T) {
	// Decode a valid address, corrupt the checksum
	addr := "tmF834qorixnCV18bVrkM8WN1Xasy5eXcZV"
	// Append a character to corrupt it
	_, err := DecodeAddress(addr+"X", "testnet3")
	require.Error(t, err)
}

func TestDecodeAddress_UnknownAddressPrefix(t *testing.T) {
	// Create an address with mainnet prefix but decode on testnet
	t1Addr := "t1XtsHnj4Ev6CWC3HfJ7Xu3GkEP7SCy8hxV"
	_, err := DecodeAddress(t1Addr, "testnet3")
	require.Error(t, err)
}

func TestDecodeAddress_ScriptHash(t *testing.T) {
	// Build a valid script hash address for mainnet
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xab}, ripemd160.Size))
	encoded, err := EncodeHash(hash[:], MainNet.ScriptHashPrefixes)
	require.NoError(t, err)

	addr, err := DecodeAddress(encoded, "mainnet")
	require.NoError(t, err)
	_, ok := addr.(*ZecAddressScriptHash)
	require.True(t, ok)
}

func TestEncodeHash_IncorrectHashLength(t *testing.T) {
	_, err := EncodeHash([]byte{1, 2, 3}, MainNet.PubHashPrefixes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "incorrect hash length")
}

func TestZecAddressPubKeyHash_String(t *testing.T) {
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xab}, ripemd160.Size))
	addr := NewAddressPubKeyHash(hash, "mainnet")
	s := addr.String()
	require.NotEmpty(t, s)
	require.Equal(t, addr.EncodeAddress(), s)
}

func TestZecAddressScriptHash_String(t *testing.T) {
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xab}, ripemd160.Size))
	addr := NewAddressScriptHash(hash, "mainnet")
	s := addr.String()
	require.NotEmpty(t, s)
	require.Equal(t, addr.EncodeAddress(), s)
}

func TestZecAddressPubKeyHash_IsForNet(t *testing.T) {
	var hash [ripemd160.Size]byte
	addr := NewAddressPubKeyHash(hash, "mainnet")

	require.True(t, addr.IsForNet(&chaincfg.Params{Name: "mainnet"}))
	require.False(t, addr.IsForNet(&chaincfg.Params{Name: "testnet3"}))
	require.False(t, addr.IsForNet(&chaincfg.Params{Name: "unknown"}))
}

func TestZecAddressScriptHash_IsForNet(t *testing.T) {
	var hash [ripemd160.Size]byte
	addr := NewAddressScriptHash(hash, "mainnet")

	require.True(t, addr.IsForNet(&chaincfg.Params{Name: "mainnet"}))
	require.False(t, addr.IsForNet(&chaincfg.Params{Name: "testnet3"}))
	require.False(t, addr.IsForNet(&chaincfg.Params{Name: "unknown"}))
}

func TestPkHashFromAddress_NilNet(t *testing.T) {
	_, err := PkHashFromAddress("t1XtsHnj4Ev6CWC3HfJ7Xu3GkEP7SCy8hxV", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil net params")
}

func TestPkHashFromAddress_InvalidAddress(t *testing.T) {
	_, err := PkHashFromAddress("invalid", &chaincfg.Params{Name: "mainnet"})
	require.Error(t, err)
}

func TestPkHashFromAddress_ScriptHash(t *testing.T) {
	// Build a valid script hash address
	var hash [ripemd160.Size]byte
	copy(hash[:], bytes.Repeat([]byte{0xab}, ripemd160.Size))
	encoded, err := EncodeHash(hash[:], MainNet.ScriptHashPrefixes)
	require.NoError(t, err)

	pkh, err := PkHashFromAddress(encoded, &chaincfg.Params{Name: "mainnet"})
	require.NoError(t, err)
	require.Equal(t, hash[:], pkh)
}

func TestDecodeTexAddress_InvalidBech32(t *testing.T) {
	_, err := DecodeAddress("tex1invalid", "mainnet")
	require.Error(t, err)
}

func TestDecodeTexAddress_TestnetPrefix(t *testing.T) {
	// textest prefix on testnet
	// We can't easily construct valid textest bech32m, so just check error for bad data
	_, err := DecodeAddress("textest1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq8eav5t", "testnet3")
	// Should error due to payload length or other validation
	require.Error(t, err)
}

func TestEncode_UnknownNet(t *testing.T) {
	wif, _ := btcutil.DecodeWIF(testWif)
	_, err := Encode(wif.PrivKey.PubKey().SerializeCompressed(), &chaincfg.Params{Name: "dummy"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown network")
}

// --- hash.go tests ---

func TestBlake2bHash(t *testing.T) {
	data := []byte("test data")
	key := []byte("ZcashPrevoutHash")
	h, err := blake2bHash(data, key)
	require.NoError(t, err)
	require.NotEqual(t, chainhash.Hash{}, h)
}

// --- WriteTxOut ---

func TestWriteTxOut(t *testing.T) {
	var buf bytes.Buffer
	txOut := wire.NewTxOut(50000, []byte{0x76, 0xa9, 0x14})
	err := WriteTxOut(&buf, 0, 3, txOut)
	require.NoError(t, err)
	require.True(t, buf.Len() > 0)
}

func TestWriteTxOut_WriterError(t *testing.T) {
	txOut := wire.NewTxOut(50000, []byte{0x76})
	err := WriteTxOut(&errWriter{}, 0, 3, txOut)
	require.Error(t, err)
}

// --- PutUint methods with errWriter ---

func TestPutUint16_Error(t *testing.T) {
	err := binarySerializer.PutUint16(&errWriter{}, littleEndian, 0x1234)
	require.Error(t, err)
}

func TestPutUint32_Error(t *testing.T) {
	err := binarySerializer.PutUint32(&errWriter{}, littleEndian, 0x12345678)
	require.Error(t, err)
}

func TestPutUint64_Error(t *testing.T) {
	err := binarySerializer.PutUint64(&errWriter{}, littleEndian, 0x123456789ABCDEF0)
	require.Error(t, err)
}

func TestPutUint8_Error(t *testing.T) {
	err := binarySerializer.PutUint8(&errWriter{}, 0x12)
	require.Error(t, err)
}

// --- Sign error paths ---

func TestSignTxOutput_PubKeyHashError(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	// Create a P2PKH script
	pkScript, _ := hex.DecodeString("76a914aefaebf9c83deba2ec76e080e2cec850dec161b188ac")

	// KeyDB that returns error
	_, err := SignTxOutput(
		&chaincfg.TestNet3Params,
		zecTx,
		0,
		pkScript,
		txscript.SigHashAll,
		txscript.KeyClosure(func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
			return nil, false, errors.New("key not found")
		}),
		nil,
		nil,
		100000,
	)
	require.Error(t, err)
}

func TestSignTxOutput_NonStandardScript(t *testing.T) {
	ph, _ := chainhash.NewHashFromStr("e446be46fe7b44de1baf3b451227da8bbabc96b27ba17940ad759a8b6e61151c")
	newTx := wire.NewMsgTx(3)
	txIn := wire.NewTxIn(wire.NewOutPoint(ph, 0), nil, nil)
	newTx.AddTxIn(txIn)
	newTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76}))
	zecTx := &MsgTx{MsgTx: newTx, ExpiryHeight: 500}

	// Non-standard script
	_, err := SignTxOutput(
		&chaincfg.TestNet3Params,
		zecTx,
		0,
		[]byte{0xff, 0xff, 0xff}, // non-standard
		txscript.SigHashAll,
		txscript.KeyClosure(func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
			return nil, false, nil
		}),
		nil,
		nil,
		100000,
	)
	require.Error(t, err)
}

// --- helper ---

type errWriter struct{}

func (e *errWriter) Write(p []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
