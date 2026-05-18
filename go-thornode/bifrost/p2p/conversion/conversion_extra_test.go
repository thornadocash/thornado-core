package conversion

import (
	"encoding/json"
	"math/big"

	"github.com/binance-chain/tss-lib/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/decred/dcrd/dcrec/edwards/v2"
	crypto2 "github.com/libp2p/go-libp2p-core/crypto"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"
)

type ConversionExtraTestSuite struct{}

var _ = Suite(&ConversionExtraTestSuite{})

func (s *ConversionExtraTestSuite) SetUpSuite(c *C) {
	SetupBech32Prefix()
}

// ── GetPreviousKeySignUicast ─────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetPreviousKeySignUicast(c *C) {
	// Suffix matches KEYSIGN1b → returns KEYSIGN1aUnicast
	c.Assert(GetPreviousKeySignUicast("round-"+messages.KEYSIGN1b), Equals, messages.KEYSIGN1aUnicast)

	// Exact match
	c.Assert(GetPreviousKeySignUicast(messages.KEYSIGN1b), Equals, messages.KEYSIGN1aUnicast)

	// Any other string → returns KEYSIGN2Unicast
	c.Assert(GetPreviousKeySignUicast("somethingelse"), Equals, messages.KEYSIGN2Unicast)
	c.Assert(GetPreviousKeySignUicast(""), Equals, messages.KEYSIGN2Unicast)
	c.Assert(GetPreviousKeySignUicast(messages.KEYSIGN3), Equals, messages.KEYSIGN2Unicast)
}

// ── GetTssPubKeyEDDSA ───────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetTssPubKeyEDDSA_NilPoint(c *C) {
	pk, addr, err := GetTssPubKeyEDDSA(nil)
	c.Assert(err, NotNil)
	c.Assert(pk, Equals, "")
	c.Assert(addr, IsNil)
}

func (s *ConversionExtraTestSuite) TestGetTssPubKeyEDDSA_InvalidPoint(c *C) {
	// Use a point that is NOT on the Edwards curve
	invalidPoint := crypto.NewECPointNoCurveCheck(edwards.Edwards(), big.NewInt(1), big.NewInt(2))
	pk, addr, err := GetTssPubKeyEDDSA(invalidPoint)
	c.Assert(err, NotNil)
	c.Assert(pk, Equals, "")
	c.Assert(addr, IsNil)
}

func (s *ConversionExtraTestSuite) TestGetTssPubKeyEDDSA_ValidPoint(c *C) {
	// Generate a real ed25519 key pair to get a valid point on the Edwards curve
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	edPub, err := edwards.ParsePubKey(pubKey.Bytes())
	c.Assert(err, IsNil)

	point, err := crypto.NewECPoint(edwards.Edwards(), edPub.X, edPub.Y)
	c.Assert(err, IsNil)

	pk, addr, err := GetTssPubKeyEDDSA(point)
	c.Assert(err, IsNil)
	c.Assert(pk, Not(Equals), "")
	c.Assert(addr, NotNil)
}

// ── BytesToHashString ────────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestBytesToHashString(c *C) {
	hash, err := BytesToHashString([]byte("hello"))
	c.Assert(err, IsNil)
	c.Assert(hash, Equals, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")

	// Empty input still works
	hash, err = BytesToHashString([]byte{})
	c.Assert(err, IsNil)
	c.Assert(hash, Not(Equals), "")

	// Nil input still works
	hash, err = BytesToHashString(nil)
	c.Assert(err, IsNil)
	c.Assert(hash, Not(Equals), "")
}

// ── GetThreshold ─────────────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetThreshold(c *C) {
	// Negative input returns error
	_, err := GetThreshold(-1)
	c.Assert(err, NotNil)

	// 0 → ceil(0)-1 = -1
	th, err := GetThreshold(0)
	c.Assert(err, IsNil)
	c.Assert(th, Equals, -1)

	// 1 → ceil(2/3)-1 = 0
	th, err = GetThreshold(1)
	c.Assert(err, IsNil)
	c.Assert(th, Equals, 0)

	// 3 → ceil(6/3)-1 = 1
	th, err = GetThreshold(3)
	c.Assert(err, IsNil)
	c.Assert(th, Equals, 1)

	// 4 → ceil(8/3)-1 = 2
	th, err = GetThreshold(4)
	c.Assert(err, IsNil)
	c.Assert(th, Equals, 2)
}

// ── GetEDDSAPrivateKeyRawBytes ──────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetEDDSAPrivateKeyRawBytes(c *C) {
	// Generate a libp2p ed25519 private key
	libp2pPriv, _, err := crypto2.GenerateEd25519Key(nil)
	c.Assert(err, IsNil)

	raw, err := GetEDDSAPrivateKeyRawBytes(libp2pPriv)
	c.Assert(err, IsNil)
	c.Assert(len(raw) > 0, Equals, true)
}

// ── GetPeerIDsFromPubKeys ───────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetPeerIDsFromPubKeys(c *C) {
	pubKeys := []string{
		"thorpub1addwnpepqtctt9l4fddeh0krvdpxmqsxa5z9xsa0ac6frqfhm9fq6c6u5lck5s8fm4n",
		"thorpub1addwnpepqga5cupfejfhtw507sh36fvwaekyjt5kwaw0cmgnpku0at2a87qqkp60t43",
	}
	peerIDs, err := GetPeerIDsFromPubKeys(pubKeys)
	c.Assert(err, IsNil)
	c.Assert(peerIDs, HasLen, 2)
	c.Assert(peerIDs[0].String(), Equals, "16Uiu2HAmBdJRswX94UwYj6VLhh4GeUf9X3SjBRgTqFkeEMLmfk2M")
	c.Assert(peerIDs[1].String(), Equals, "16Uiu2HAkyR9dsFqkj1BqKw8ZHAUU2yur6ZLRJxPTiiVYP5uBMeMG")

	// Error case: invalid pub key
	_, err = GetPeerIDsFromPubKeys([]string{"invalid"})
	c.Assert(err, NotNil)

	// Empty input
	peerIDs, err = GetPeerIDsFromPubKeys(nil)
	c.Assert(err, IsNil)
	c.Assert(peerIDs, IsNil)
}

// ── GetPubKeyFromPeerID ─────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetPubKeyFromPeerID(c *C) {
	pk, err := GetPubKeyFromPeerID("16Uiu2HAmBdJRswX94UwYj6VLhh4GeUf9X3SjBRgTqFkeEMLmfk2M")
	c.Assert(err, IsNil)
	c.Assert(pk, Equals, "thorpub1addwnpepqtctt9l4fddeh0krvdpxmqsxa5z9xsa0ac6frqfhm9fq6c6u5lck5s8fm4n")

	// Invalid peer ID
	_, err = GetPubKeyFromPeerID("invalidpeerid")
	c.Assert(err, NotNil)
}

// ── GetPriKeyRawBytes error path ────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetPriKeyRawBytes_NotSecp256k1(c *C) {
	// ed25519 key should fail the type assertion
	edKey := ed25519.GenPrivKey()
	_, err := GetPriKeyRawBytes(edKey)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "private key is not secp256p1.PrivKey")
}

// ── RandStringBytesMask ─────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestRandStringBytesMask(c *C) {
	result := RandStringBytesMask(10)
	c.Assert(len(result), Equals, 10)

	// 0-length
	result = RandStringBytesMask(0)
	c.Assert(result, Equals, "")
}

// ── VersionLTCheck ──────────────────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestVersionLTCheck(c *C) {
	// currentVer < expectedVer
	lt, err := VersionLTCheck("0.13.0", "0.14.0")
	c.Assert(err, IsNil)
	c.Assert(lt, Equals, true)

	// currentVer == expectedVer
	lt, err = VersionLTCheck("0.14.0", "0.14.0")
	c.Assert(err, IsNil)
	c.Assert(lt, Equals, false)

	// currentVer > expectedVer
	lt, err = VersionLTCheck("0.15.0", "0.14.0")
	c.Assert(err, IsNil)
	c.Assert(lt, Equals, false)

	// Invalid expected version
	_, err = VersionLTCheck("0.14.0", "invalid")
	c.Assert(err, NotNil)

	// Invalid current version
	_, err = VersionLTCheck("invalid", "0.14.0")
	c.Assert(err, NotNil)
}

// ── GetRandomPubKey / GetRandomPeerID ───────────────────────────────────

func (s *ConversionExtraTestSuite) TestGetRandomPubKey(c *C) {
	pk := GetRandomPubKey()
	c.Assert(pk, Not(Equals), "")
}

func (s *ConversionExtraTestSuite) TestGetRandomPeerID(c *C) {
	// GetRandomPeerID generates a peer ID from a random key; just ensure it doesn't panic
	_ = GetRandomPeerID()
}

// ── AccPubKeysFromPartyIDs error path ───────────────────────────────────

func (s *ConversionExtraTestSuite) TestAccPubKeysFromPartyIDs_NotFound(c *C) {
	// Asking for party ID that doesn't exist in map
	_, err := AccPubKeysFromPartyIDs([]string{"nonexistent"}, nil)
	c.Assert(err, NotNil)
}

// ── CheckKeyOnCurve ed25519 path ────────────────────────────────────────

func (s *ConversionExtraTestSuite) TestCheckKeyOnCurve_Ed25519(c *C) {
	// Generate an ed25519 key and get its EDDSA pub key string
	privKey := ed25519.GenPrivKey()
	pubKey := privKey.PubKey()
	edPub, err := edwards.ParsePubKey(pubKey.Bytes())
	c.Assert(err, IsNil)

	point, err := crypto.NewECPoint(edwards.Edwards(), edPub.X, edPub.Y)
	c.Assert(err, IsNil)

	pk, _, err := GetTssPubKeyEDDSA(point)
	c.Assert(err, IsNil)

	onCurve, err := CheckKeyOnCurve(pk)
	c.Assert(err, IsNil)
	c.Assert(onCurve, Equals, true)
}

// ── GetTssPubKeyECDSA with JSON-deserialized point ──────────────────────

func (s *ConversionExtraTestSuite) TestGetTssPubKeyECDSA_WithJSONPoint(c *C) {
	var point crypto.ECPoint
	c.Assert(json.Unmarshal([]byte(`{"Coords":[70074650318631491136896111706876206496089700125696166275258483716815143842813,72125378038650252881868972131323661098816214918201601489154946637636730727892],"CurveType":"ecdsa"}`), &point), IsNil)
	pk, addr, err := GetTssPubKeyECDSA(&point)
	c.Assert(err, IsNil)
	c.Assert(pk, Equals, "thorpub1addwnpepq2dwek9hkrlxjxadrlmy9fr42gqyq6029q0hked46l3u6a9fxqel6tma5eu")
	c.Assert(addr.String(), Equals, "thor17l7cyxqzg4xymnl0alrhqwja276s3rns3fjdvm")
}
