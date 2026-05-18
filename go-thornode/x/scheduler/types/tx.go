package types

import (
	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgScheduleExecuteContract{}

// NewMsgScheduleExecuteContract creates a msg to create a new denom
func NewMsgScheduleExecuteContract(sender string, after uint64, msg []byte) *MsgScheduleExecuteContract {
	return &MsgScheduleExecuteContract{
		Sender: sender,
		After:  after,
		Msg:    msg,
	}
}

func (m MsgScheduleExecuteContract) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(m.Sender)
	if err != nil {
		return errors.Wrapf(sdkerrors.ErrInvalidAddress, "Invalid sender address (%s)", err)
	}

	return nil
}

func (m MsgScheduleExecuteContract) GetSigners() []sdk.AccAddress {
	sender, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{sender}
}
