package ed25519

import (
	"testing"

	"github.com/mr-tron/base58"
	. "gopkg.in/check.v1"
)

func TestEd25519(t *testing.T) { TestingT(t) }

type Ed25519Suite struct{}

var _ = Suite(&Ed25519Suite{})

func (s *Ed25519Suite) TestAlgo(c *C) {
	mnemonic := "dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog fossil"

	// 64 bytes private key
	pk, err := Ed25519.Derive()(mnemonic, "", HDPath)
	c.Assert(err, IsNil)
	c.Assert(pk, NotNil)
	c.Assert(len(pk), Equals, 64)

	// 64 bytes cosmos.ed25519 private key
	privKey := Ed25519.Generate()(pk)
	b := privKey.Bytes()
	c.Assert(b, NotNil)
	c.Assert(len(b), Equals, 64)
	addr := base58.Encode(privKey.PubKey().Bytes())
	c.Assert(addr, Equals, "BJvLwohMSn9iL7x5oV7tc1eGH8D6qUiMd8ip6eVCtF73")
}
