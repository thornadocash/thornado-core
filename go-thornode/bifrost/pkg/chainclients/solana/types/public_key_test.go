package types

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type PubKeyTestSuite struct{}

var _ = Suite(&PubKeyTestSuite{})

func (s *PubKeyTestSuite) TestPubKeyFromString(c *C) {
	pkString := "BEcrPLugbAY1zEJGUNYgzcsxMi72rPeVwG6qKm96LK5g"
	pk, err := PublicKeyFromString(pkString)
	c.Assert(err, IsNil)
	c.Assert(pk.String(), Equals, "BEcrPLugbAY1zEJGUNYgzcsxMi72rPeVwG6qKm96LK5g")
}

func (s *PubKeyTestSuite) TestPubKeyFromStringError(c *C) {
	_, err := PublicKeyFromString("not-valid-base58!!!")
	c.Assert(err, NotNil)
}

func (s *PubKeyTestSuite) TestPubKeyBytes(c *C) {
	pk := MustPublicKeyFromString("BEcrPLugbAY1zEJGUNYgzcsxMi72rPeVwG6qKm96LK5g")
	b := pk.Bytes()
	c.Assert(len(b), Equals, PublicKeyLength)
	// Round-trip: PublicKeyFromBytes should produce same key
	pk2 := PublicKeyFromBytes(b)
	c.Assert(pk2, DeepEquals, pk)
}

func (s *PubKeyTestSuite) TestPubKeyEqualsString(c *C) {
	pkString := "BEcrPLugbAY1zEJGUNYgzcsxMi72rPeVwG6qKm96LK5g"
	pk := MustPublicKeyFromString(pkString)
	c.Assert(pk.EqualsString(pkString), Equals, true)
	c.Assert(pk.EqualsString("11111111111111111111111111111111"), Equals, false)
}

func (s *PubKeyTestSuite) TestPublicKeyFromBytesLong(c *C) {
	// Bytes longer than PublicKeyLength should be truncated
	longBytes := make([]byte, 64)
	for i := range longBytes {
		longBytes[i] = byte(i)
	}
	pk := PublicKeyFromBytes(longBytes)
	// Should only use first 32 bytes
	c.Assert(pk[:], DeepEquals, longBytes[:PublicKeyLength])
}

func (s *PubKeyTestSuite) TestPublicKeyFromBytesShort(c *C) {
	// Bytes shorter than PublicKeyLength should be right-aligned (zero-padded on the left)
	shortBytes := []byte{1, 2, 3}
	pk := PublicKeyFromBytes(shortBytes)
	expected := make([]byte, PublicKeyLength)
	expected[29] = 1
	expected[30] = 2
	expected[31] = 3
	c.Assert(pk[:], DeepEquals, expected)
}
