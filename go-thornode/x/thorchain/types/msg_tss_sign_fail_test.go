package types

import (
	"errors"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	tssMessages "gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MsgTssKeysignFailSuite struct{}

var _ = Suite(&MsgTssKeysignFailSuite{})

func (s MsgTssKeysignFailSuite) TestMsgTssKeysignFail(c *C) {
	b := Blame{
		FailReason: "fail to TSS sign",
		Round:      tssMessages.KEYSIGN1aUnicast,
		BlameNodes: []Node{
			{Pubkey: GetRandomPubKey().String()},
			{Pubkey: GetRandomPubKey().String()},
		},
	}
	coins := common.Coins{
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100)),
	}
	msg, err := NewMsgTssKeysignFail(1, b, "hello", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(err, IsNil)
	EnsureMsgBasicCorrect(msg, c)
	// Empty blame (no round) should succeed at construction.
	m, err := NewMsgTssKeysignFail(1, Blame{}, "hello", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	m, err = NewMsgTssKeysignFail(1, b, "", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	m, err = NewMsgTssKeysignFail(1, b, "hello", common.Coins{}, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	m, err = NewMsgTssKeysignFail(1, b, "hello", common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
		common.NewCoin(common.EmptyAsset, cosmos.ZeroUint()),
	}, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	m, err = NewMsgTssKeysignFail(1, b, "hello", coins, cosmos.AccAddress{}, GetRandomPubKey())
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	msg2, err := NewMsgTssKeysignFail(1, b, "hello", coins, cosmos.AccAddress{}, GetRandomPubKey())
	c.Assert(err, IsNil)
	err2 := msg2.ValidateBasic()
	c.Check(err2, NotNil)
	c.Check(errors.Is(err2, se.ErrInvalidAddress), Equals, true)

	msg3, err := NewMsgTssKeysignFail(1, b, "hello", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(err, IsNil)
	msg3.ID = ""
	err3 := msg3.ValidateBasic()
	c.Check(err3, NotNil)
	c.Check(errors.Is(err3, se.ErrUnknownRequest), Equals, true)

	// Empty blame (with empty round) should succeed at construction.
	msg4, err := NewMsgTssKeysignFail(1, Blame{}, "hello", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(err, IsNil)
	c.Assert(msg4, NotNil)

	// Verify invalid coins are caught by ValidateBasic.
	msg4b, err := NewMsgTssKeysignFail(1, b, "hello", coins, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(err, IsNil)
	msg4b.Coins = append(msg4b.Coins, common.NewCoin(common.EmptyAsset, cosmos.ZeroUint()))
	err4b := msg4b.ValidateBasic()
	c.Check(err4b, NotNil)
	c.Check(errors.Is(err4b, se.ErrInvalidCoins), Equals, true)

	msg5, err := NewMsgTssKeysignFail(1, b, "hello", common.Coins{}, GetRandomBech32Addr(), GetRandomPubKey())
	c.Assert(err, IsNil)
	err5 := msg5.ValidateBasic()
	c.Check(err5, NotNil)
	c.Check(errors.Is(err5, se.ErrUnknownRequest), Equals, true)

	msg6, err := NewMsgTssKeysignFail(1, b, "hello", coins, GetRandomBech32Addr(), common.EmptyPubKey)
	c.Assert(err, IsNil)
	err6 := msg6.ValidateBasic()
	c.Check(err6, NotNil)
	c.Check(errors.Is(err6, se.ErrUnknownRequest), Equals, true)
}

// TestMsgTssKeysignFailRoundNormalization verifies that constructors canonicalize
// outgoing rounds while on-chain validation only accepts canonical names.
func (s MsgTssKeysignFailSuite) TestMsgTssKeysignFailRoundNormalization(c *C) {
	blame := Blame{
		FailReason: "fail to TSS sign",
		Round:      "thorchain.tsslib.ecdsa.signing.SignRound7Message",
		BlameNodes: []Node{
			{Pubkey: GetRandomPubKey().String()},
			{Pubkey: GetRandomPubKey().String()},
		},
	}
	coins := common.Coins{
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100)),
	}
	signer := GetRandomBech32Addr()
	pubKey := GetRandomPubKey()

	msgRaw, err := NewMsgTssKeysignFail(1, blame, "hello", coins, signer, pubKey)
	c.Assert(err, IsNil)
	c.Check(msgRaw.Blame.Round, Equals, tssMessages.KEYSIGN7)

	blame.Round = tssMessages.KEYSIGN7
	msgCanonical, err := NewMsgTssKeysignFail(1, blame, "hello", coins, signer, pubKey)
	c.Assert(err, IsNil)
	c.Check(msgRaw.ID, Equals, msgCanonical.ID)

	msgCanonical.Blame.Round = "thorchain.tsslib.ecdsa.signing.SignRound7Message"
	err = msgCanonical.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	msgCanonical.Blame.Round = "invalid-round"
	err = msgCanonical.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// Empty round is allowed (e.g. blame with no specific nodes).
	msgCanonical.Blame.Round = ""
	err = msgCanonical.ValidateBasic()
	c.Check(err, IsNil)
}
