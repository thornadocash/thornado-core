package tron

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/config"

	. "gopkg.in/check.v1"
)

type KeyManagerTestSuite struct {
	keys *thorclient.Keys
}

var _ = Suite(&KeyManagerTestSuite{})

func (s *KeyManagerTestSuite) SetUpSuite(c *C) {
	m = GetMetricForTest(c)
	c.Assert(m, NotNil)

	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.keys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	c.Assert(s.keys, NotNil)
}

func (s *KeyManagerTestSuite) TestLocalKeyManager(c *C) {
	keyManager, err := NewLocalKeyManager(s.keys)
	c.Assert(err, IsNil)

	commonPub := keyManager.Pubkey()
	c.Assert(commonPub, NotNil)

	btcecPub, err := commonPub.Secp256K1()
	c.Assert(err, IsNil)

	ecdsaPub := &ecdsa.PublicKey{
		Curve: secp256k1.S256(),
		X:     btcecPub.X,
		Y:     btcecPub.Y,
	}

	hash := sha256.Sum256([]byte("hello world"))
	signature, err := keyManager.Sign(hash[:])
	c.Assert(err, IsNil)
	c.Assert(signature, NotNil)

	R := new(big.Int).SetBytes(signature[:32])
	S := new(big.Int).SetBytes(signature[32:64])

	ok := ecdsa.Verify(ecdsaPub, hash[:], R, S)
	c.Assert(ok, Equals, true)
}
