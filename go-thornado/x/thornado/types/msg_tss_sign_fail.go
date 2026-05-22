package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgTssKeysignFail{}
	_ sdk.HasValidateBasic = &MsgTssKeysignFail{}
	_ sdk.LegacyMsg        = &MsgTssKeysignFail{}
)

// NewMsgTssKeysignFail create a new instance of MsgTssKeysignFail message
func NewMsgTssKeysignFail(height int64, blame Blame, memo string, coins common.Coins, signer cosmos.AccAddress, pubKey common.PubKey) (*MsgTssKeysignFail, error) {
	// Normalize the round before hashing so outgoing messages always use the
	// canonical wire format and stable consensus ID.
	round, err := NormalizeTssKeysignRound(blame.Round)
	if err != nil {
		return nil, fmt.Errorf("fail to normalize keysign fail round: %w", err)
	}
	blame.Round = round
	id, err := getMsgTssKeysignFailID(blame.BlameNodes, height, memo, coins, pubKey, blame.Round)
	if err != nil {
		return nil, fmt.Errorf("fail to get keysign fail id:%w", err)
	}
	return &MsgTssKeysignFail{
		ID:     id,
		Height: height,
		Blame:  blame,
		Memo:   memo,
		Coins:  coins,
		Signer: signer,
		PubKey: pubKey,
	}, nil
}

// getTssKeysignFailID this method will use all the members that caused the tss
// keysign failure , as well as the block height of the txout item to generate
// a hash, given that , if the same party keep failing the same txout item ,
// then we will only slash it once.
func getMsgTssKeysignFailID(members []Node, height int64, memo string, coins common.Coins, pubKey common.PubKey, round string) (string, error) {
	// ensure input pubkeys list is deterministically sorted
	sort.SliceStable(members, func(i, j int) bool {
		return members[i].Pubkey < members[j].Pubkey
	})
	sb := strings.Builder{}
	for _, item := range members {
		sb.WriteString(item.Pubkey)
	}
	sb.WriteString(fmt.Sprintf("%d", height))
	sb.WriteString(memo)
	sb.WriteString(pubKey.String())
	for _, c := range coins {
		sb.WriteString(c.String())
	}
	sb.WriteString(round)
	hash := sha256.New()
	_, err := hash.Write([]byte(sb.String()))
	if err != nil {
		return "", fmt.Errorf("fail to create hash id,err:%w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgTssKeysignFail) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if len(m.ID) == 0 {
		return cosmos.ErrUnknownRequest("ID cannot be blank")
	}
	if len(m.Coins) == 0 {
		return cosmos.ErrUnknownRequest("no coins")
	}
	if err := m.Coins.Valid(); err != nil {
		return cosmos.ErrInvalidCoins(err.Error())
	}
	if m.Blame.IsEmpty() {
		return cosmos.ErrUnknownRequest("tss blame is empty")
	}
	for _, node := range m.Blame.BlameNodes {
		if _, err := common.NewPubKey(node.Pubkey); err != nil {
			return cosmos.ErrUnknownRequest("invalid blame node pubkey: " + node.Pubkey)
		}
	}
	if m.Height <= 0 {
		return cosmos.ErrUnknownRequest("block height cannot be equal to less than zero")
	}
	if m.Memo != "" {
		return cosmos.ErrUnknownRequest("keysign failure memo is disabled")
	}
	if m.PubKey.IsEmpty() {
		return cosmos.ErrUnknownRequest("vault pubkey cannot be empty")
	}
	// Enforce canonical round names on-chain so handler logic can safely use the
	// incoming round value without re-normalizing it.
	round, err := NormalizeTssKeysignRound(m.Blame.Round)
	if err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if round != m.Blame.Round {
		return cosmos.ErrUnknownRequest("blame round must use canonical name")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTssKeysignFail) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgTssKeysignFailCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgTssKeysignFail)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgTssKeysignFail: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
