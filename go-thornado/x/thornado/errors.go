package thornado

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/hashicorp/go-multierror"
)

// Thornado error code start at 99
const (
	// CodeBadVersion error code for bad version
	CodeInternalError          uint32 = 99
	CodeTxFail                 uint32 = 100
	CodeBadVersion             uint32 = 101
	CodeInvalidMessage         uint32 = 102
	CodeInvalidVault           uint32 = 104
	CodeWithdrawFailValidation uint32 = 130
	CodeFailAddOutboundTx      uint32 = 131
	CodeFailSaveEvent          uint32 = 132
	CodeWithdrawWithin24Hours  uint32 = 136
	CodeWithdrawFail           uint32 = 137
	CodeInvalidChain           uint32 = 138
	CodeWithdrawLockup         uint32 = 139
)

var (
	errNotAuthorized          = fmt.Errorf("not authorized")
	errInvalidVersion         = fmt.Errorf("bad version")
	errBadVersion             = errorsmod.Register(DefaultCodespace, CodeBadVersion, errInvalidVersion.Error())
	errInvalidMessage         = errorsmod.Register(DefaultCodespace, CodeInvalidMessage, "invalid message")
	errFailSaveEvent          = errorsmod.Register(DefaultCodespace, CodeFailSaveEvent, "fail to save add events")
	errFailAddOutboundTx      = errorsmod.Register(DefaultCodespace, CodeFailAddOutboundTx, "prepare outbound tx not successful")
	errWithdrawFailValidation = errorsmod.Register(DefaultCodespace, CodeWithdrawFailValidation, "fail to validate withdraw")
	errWithdrawLockup         = errorsmod.Register(DefaultCodespace, CodeWithdrawLockup, "last add within lockup blocks")
	errWithdrawFail           = errorsmod.Register(DefaultCodespace, CodeWithdrawFail, "fail to withdraw")
	errInternal               = errorsmod.Register(DefaultCodespace, CodeInternalError, "internal error")
)

// ErrInternal return an error  of errInternal with additional message
func ErrInternal(err error, msg string) error {
	return errorsmod.Wrap(multierror.Append(errInternal, err), msg)
}
