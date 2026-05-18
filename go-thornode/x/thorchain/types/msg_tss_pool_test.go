package types

import (
	"errors"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
)

type MsgTssPoolSuite struct{}

var _ = Suite(&MsgTssPoolSuite{})

func (s *MsgTssPoolSuite) TestMsgTssPool(c *C) {
	pk := GetRandomPubKey()
	pks := []string{
		GetRandomPubKey().String(), GetRandomPubKey().String(), GetRandomPubKey().String(),
	}
	addr, err := common.PubKey(pks[0]).GetThorAddress()
	c.Assert(err, IsNil)
	keygenTime := int64(1024)
	msg, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, []string{common.RuneAsset().Chain.String()}, addr, keygenTime)
	c.Assert(err, IsNil)
	c.Assert(msg.ValidateBasic(), IsNil)
	EnsureMsgBasicCorrect(msg, c)

	chains := []string{common.RuneAsset().Chain.String()}
	m, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, nil, keygenTime)
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	c.Assert(m.ValidateBasic(), NotNil)
	m, err = NewMsgTssPool(nil, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	c.Assert(m.ValidateBasic(), NotNil)
	m, err = NewMsgTssPool(pks, "", nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	c.Assert(m.ValidateBasic(), NotNil)
	m, err = NewMsgTssPool(pks, "bogusPubkey", nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(m, NotNil)
	c.Assert(err, IsNil)
	c.Assert(m.ValidateBasic(), NotNil)

	// fails on empty chain list
	msg, err = NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, []string{}, addr, keygenTime)
	c.Assert(err, IsNil)
	c.Check(msg.ValidateBasic(), NotNil)
	// fails on duplicates in chain list
	msg, err = NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, []string{common.RuneAsset().Chain.String(), common.RuneAsset().Chain.String()}, addr, keygenTime)
	c.Assert(err, IsNil)
	c.Check(msg.ValidateBasic(), NotNil)

	msg1, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(err, IsNil)
	msg1.ID = ""
	err1 := msg1.ValidateBasic()
	c.Assert(err1, NotNil)
	c.Check(errors.Is(err1, se.ErrUnknownRequest), Equals, true)

	msg2, err := NewMsgTssPool(append(pks, ""), pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(err, IsNil)
	err2 := msg2.ValidateBasic()
	c.Assert(err2, NotNil)
	c.Check(errors.Is(err2, se.ErrUnknownRequest), Equals, true)

	var allPks []string
	for i := 0; i < 110; i++ {
		allPks = append(allPks, GetRandomPubKey().String())
	}
	msg3, err := NewMsgTssPool(allPks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nil, chains, addr, keygenTime)
	c.Assert(err, IsNil)
	err3 := msg3.ValidateBasic()
	c.Assert(err3, NotNil)
	c.Check(errors.Is(err3, se.ErrUnknownRequest), Equals, true)
}

func (s *MsgTssPoolSuite) TestMsgTssPoolBlameNodesValidation(c *C) {
	pk := GetRandomPubKey()
	pks := []string{
		GetRandomPubKey().String(), GetRandomPubKey().String(), GetRandomPubKey().String(),
	}
	addr, err := common.PubKey(pks[0]).GetThorAddress()
	c.Assert(err, IsNil)
	keygenTime := int64(1024)

	// Valid: blame nodes are keygen participants with no duplicates
	blame := []Blame{
		{
			FailReason: "test",
			BlameNodes: []Node{
				{Pubkey: pks[0]},
				{Pubkey: pks[1]},
			},
		},
	}
	msg, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, blame, nil, addr, keygenTime)
	c.Assert(err, IsNil)
	// This should fail due to missing chains, but the blame check should pass
	err = msg.ValidateBasic()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Not(Equals), "blame node not in keygen participants: unknown request")
	c.Assert(err.Error(), Not(Equals), "duplicate blame node: unknown request")

	// Invalid: blame node not in keygen participants
	nonParticipantBlame := []Blame{
		{
			FailReason: "test",
			BlameNodes: []Node{
				{Pubkey: pks[0]},
				{Pubkey: GetRandomPubKey().String()}, // not a keygen participant
			},
		},
	}
	msg2, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, nonParticipantBlame, nil, addr, keygenTime)
	c.Assert(err, IsNil)
	err = msg2.ValidateBasic()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "blame node not in keygen participants: unknown request")
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// Invalid: duplicate blame nodes within same Blame
	duplicateBlame := []Blame{
		{
			FailReason: "test",
			BlameNodes: []Node{
				{Pubkey: pks[0]},
				{Pubkey: pks[0]}, // duplicate
			},
		},
	}
	msg3, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, duplicateBlame, nil, addr, keygenTime)
	c.Assert(err, IsNil)
	err = msg3.ValidateBasic()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "duplicate blame node: unknown request")
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// Invalid: duplicate blame nodes across multiple Blame entries
	duplicateAcrossBlame := []Blame{
		{
			FailReason: "test1",
			BlameNodes: []Node{
				{Pubkey: pks[0]},
			},
		},
		{
			FailReason: "test2",
			BlameNodes: []Node{
				{Pubkey: pks[0]}, // duplicate from first Blame
			},
		},
	}
	msg4, err := NewMsgTssPool(pks, pk, nil, nil, KeygenType_AsgardKeygen, 1, duplicateAcrossBlame, nil, addr, keygenTime)
	c.Assert(err, IsNil)
	err = msg4.ValidateBasic()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "duplicate blame node: unknown request")
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
}
