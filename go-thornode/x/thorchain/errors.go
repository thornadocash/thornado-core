package thorchain

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/hashicorp/go-multierror"
)

// THORChain error code start at 99
const (
	// CodeBadVersion error code for bad version
	CodeInternalError     uint32 = 99
	CodeTxFail            uint32 = 100
	CodeBadVersion        uint32 = 101
	CodeInvalidMessage    uint32 = 102
	CodeInvalidVault      uint32 = 104
	CodeInvalidMemo       uint32 = 105
	CodeInvalidPoolStatus uint32 = 107

	CodeSwapFail                 uint32 = 108
	CodeSwapFailNotEnoughFee     uint32 = 110
	CodeSwapFailInvalidAmount    uint32 = 113
	CodeSwapFailInvalidBalance   uint32 = 114
	CodeSwapFailNotEnoughBalance uint32 = 115

	CodeAddLiquidityFailValidation   uint32 = 120
	CodeFailGetLiquidityProvider     uint32 = 122
	CodeAddLiquidityMismatchAddr     uint32 = 123
	CodeLiquidityInvalidPoolAsset    uint32 = 124
	CodeAddLiquidityRUNEOverLimit    uint32 = 125
	CodeAddLiquidityRUNEMoreThanBond uint32 = 126

	CodeWithdrawFailValidation   uint32 = 130
	CodeFailAddOutboundTx        uint32 = 131
	CodeFailSaveEvent            uint32 = 132
	CodeNoLiquidityUnitLeft      uint32 = 135
	CodeWithdrawWithin24Hours    uint32 = 136
	CodeWithdrawFail             uint32 = 137
	CodeEmptyChain               uint32 = 138
	CodeWithdrawLockup           uint32 = 139
	CodeTCYClaimFailValidation   uint32 = 140
	CodeTCYStakeFailValidation   uint32 = 141
	CodeTCYUnstakeFailValidation uint32 = 142
)

var (
	errNotAuthorized                = fmt.Errorf("not authorized")
	errInvalidVersion               = fmt.Errorf("bad version")
	errBadVersion                   = errorsmod.Register(DefaultCodespace, CodeBadVersion, errInvalidVersion.Error())
	errInvalidMessage               = errorsmod.Register(DefaultCodespace, CodeInvalidMessage, "invalid message")
	errInvalidMemo                  = errorsmod.Register(DefaultCodespace, CodeInvalidMemo, "invalid memo")
	errFailSaveEvent                = errorsmod.Register(DefaultCodespace, CodeFailSaveEvent, "fail to save add events")
	errAddLiquidityFailValidation   = errorsmod.Register(DefaultCodespace, CodeAddLiquidityFailValidation, "fail to validate add liquidity")
	errAddLiquidityRUNEOverLimit    = errorsmod.Register(DefaultCodespace, CodeAddLiquidityRUNEOverLimit, "add liquidity rune is over limit")
	errAddLiquidityRUNEMoreThanBond = errorsmod.Register(DefaultCodespace, CodeAddLiquidityRUNEMoreThanBond, "add liquidity rune is more than bond")
	errInvalidPoolStatus            = errorsmod.Register(DefaultCodespace, CodeInvalidPoolStatus, "invalid pool status")
	errFailAddOutboundTx            = errorsmod.Register(DefaultCodespace, CodeFailAddOutboundTx, "prepare outbound tx not successful")
	errWithdrawFailValidation       = errorsmod.Register(DefaultCodespace, CodeWithdrawFailValidation, "fail to validate withdraw")
	errFailGetLiquidityProvider     = errorsmod.Register(DefaultCodespace, CodeFailGetLiquidityProvider, "fail to get liquidity provider")
	errAddLiquidityMismatchAddr     = errorsmod.Register(DefaultCodespace, CodeAddLiquidityMismatchAddr, "memo paired address must be non-empty and together with origin address match the liquidity provider record")
	errSwapFailInvalidAmount        = errorsmod.Register(DefaultCodespace, CodeSwapFailInvalidAmount, "fail swap, invalid amount")
	errSwapFailInvalidBalance       = errorsmod.Register(DefaultCodespace, CodeSwapFailInvalidBalance, "fail swap, invalid balance")
	errSwapFailNotEnoughBalance     = errorsmod.Register(DefaultCodespace, CodeSwapFailNotEnoughBalance, "fail swap, not enough balance")
	errNoLiquidityUnitLeft          = errorsmod.Register(DefaultCodespace, CodeNoLiquidityUnitLeft, "nothing to withdraw")
	errWithdrawLockup               = errorsmod.Register(DefaultCodespace, CodeWithdrawLockup, "last add within lockup blocks")
	errWithdrawFail                 = errorsmod.Register(DefaultCodespace, CodeWithdrawFail, "fail to withdraw")
	errInternal                     = errorsmod.Register(DefaultCodespace, CodeInternalError, "internal error")
	errTCYClaimFailValidation       = errorsmod.Register(DefaultCodespace, CodeTCYClaimFailValidation, "fail to validate tcy claim")
	errTCYStakeFailValidation       = errorsmod.Register(DefaultCodespace, CodeTCYStakeFailValidation, "fail to validate tcy stake")
	errTCYUnstakeFailValidation     = errorsmod.Register(DefaultCodespace, CodeTCYUnstakeFailValidation, "fail to validate tcy unstake")
)

// ErrInternal return an error  of errInternal with additional message
func ErrInternal(err error, msg string) error {
	return errorsmod.Wrap(multierror.Append(errInternal, err), msg)
}
