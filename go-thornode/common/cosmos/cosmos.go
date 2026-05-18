package cosmos

import (
	"bufio"
	"bytes"
	"fmt"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint SA1019 deprecated
	se "github.com/cosmos/cosmos-sdk/types/errors"
	"gitlab.com/thorchain/thornode/v3/common/crypto/ed25519"
)

const (
	DefaultCoinDecimals = 8

	EnvSignerName     = "SIGNER_NAME"
	EnvSignerPassword = "SIGNER_PASSWD"
	EnvChainHome      = "CHAIN_HOME_FOLDER"
)

var (
	KeyringServiceName           = sdk.KeyringServiceName
	NewKVStoreKeys               = storetypes.NewKVStoreKeys
	NewMemoryStoreKeys           = storetypes.NewMemoryStoreKeys
	NewUint                      = sdkmath.NewUint
	ParseUint                    = sdkmath.ParseUint
	NewInt                       = sdkmath.NewInt
	NewDec                       = sdkmath.LegacyNewDec
	ZeroInt                      = sdkmath.ZeroInt
	ZeroUint                     = sdkmath.ZeroUint
	ZeroDec                      = sdkmath.LegacyZeroDec
	OneUint                      = sdkmath.OneUint
	NewCoin                      = sdk.NewCoin
	NewCoins                     = sdk.NewCoins
	ParseCoins                   = sdk.ParseCoinsNormalized
	NewDecWithPrec               = sdkmath.LegacyNewDecWithPrec
	NewDecFromBigInt             = sdkmath.LegacyNewDecFromBigInt
	NewIntFromBigInt             = sdkmath.NewIntFromBigInt
	NewUintFromBigInt            = sdkmath.NewUintFromBigInt
	AccAddressFromBech32         = sdk.AccAddressFromBech32
	MustAccAddressFromBech32     = sdk.MustAccAddressFromBech32
	AccAddressFromHexUnsafe      = sdk.AccAddressFromHexUnsafe
	VerifyAddressFormat          = sdk.VerifyAddressFormat
	GetFromBech32                = sdk.GetFromBech32
	NewAttribute                 = sdk.NewAttribute
	NewDecFromStr                = sdkmath.LegacyNewDecFromStr
	GetConfig                    = sdk.GetConfig
	NewEvent                     = sdk.NewEvent
	RegisterLegacyAminoCodec     = sdk.RegisterLegacyAminoCodec
	NewEventManager              = sdk.NewEventManager
	EventTypeMessage             = sdk.EventTypeMessage
	AttributeKeyModule           = sdk.AttributeKeyModule
	KVStorePrefixIterator        = storetypes.KVStorePrefixIterator
	KVStoreReversePrefixIterator = storetypes.KVStoreReversePrefixIterator
	NewKVStoreKey                = storetypes.NewKVStoreKey
	NewMemoryStoreKey            = storetypes.NewMemoryStoreKey
	NewTransientStoreKey         = storetypes.NewTransientStoreKey
	StoreTypeTransient           = storetypes.StoreTypeTransient
	StoreTypeIAVL                = storetypes.StoreTypeIAVL
	NewContext                   = sdk.NewContext
	NewUintFromString            = sdkmath.NewUintFromString

	GetPubKeyFromBech32     = legacybech32.UnmarshalPubKey // nolint SA1019 deprecated
	Bech32ifyPubKey         = legacybech32.MarshalPubKey   // nolint SA1019 deprecated
	Bech32PubKeyTypeConsPub = legacybech32.ConsPK
	Bech32PubKeyTypeAccPub  = legacybech32.AccPK
	MustSortJSON            = sdk.MustSortJSON
	CodeUnauthorized        = uint32(4)
	CodeInsufficientFunds   = uint32(5)
)

type (
	Context    = sdk.Context
	Uint       = sdkmath.Uint
	Int        = sdkmath.Int
	Coin       = sdk.Coin
	Coins      = sdk.Coins
	AccAddress = sdk.AccAddress
	Attribute  = sdk.Attribute
	Result     = sdk.Result
	Event      = sdk.Event
	Events     = sdk.Events
	Dec        = sdkmath.LegacyDec
	Msg        = sdk.Msg
	Iterator   = storetypes.Iterator
	StoreKey   = storetypes.StoreKey
	TxResponse = sdk.TxResponse
	Account    = sdk.AccountI
)

// Handler defines the core of the state transition function of an application.
type Handler func(ctx Context, msg Msg) (*Result, error)

var _ sdk.Address = AccAddress{}

func ErrUnknownRequest(msg string) error {
	return se.ErrUnknownRequest.Wrap(msg)
}

func ErrInvalidAddress(addr string) error {
	return se.ErrInvalidAddress.Wrap(addr)
}

func ErrInvalidCoins(msg string) error {
	return se.ErrInvalidCoins.Wrap(msg)
}

func ErrUnauthorized(msg string) error {
	return se.ErrUnauthorized.Wrap(msg)
}

// RoundToDecimal round the given amt to the desire decimals
func RoundToDecimal(amt Uint, dec int64) Uint {
	if dec != 0 && dec < DefaultCoinDecimals {
		prec := DefaultCoinDecimals - dec
		if prec == 0 { // sanity check
			return amt
		}
		precisionAdjust := sdkmath.NewUintFromBigInt(big.NewInt(0).Exp(big.NewInt(10), big.NewInt(prec), nil))
		amt = amt.Quo(precisionAdjust).Mul(precisionAdjust)
	}
	return amt
}

// KeybaseStore to store keys
type KeybaseStore struct {
	Keybase      ckeys.Keyring
	SignerName   string
	SignerPasswd string
}

func SignerCreds() (string, string) {
	var username, password string
	reader := bufio.NewReader(os.Stdin)

	if signerName := os.Getenv(EnvSignerName); signerName != "" {
		username = signerName
	} else {
		username, _ = input.GetString("Enter Signer name:", reader)
	}

	if signerPassword := os.Getenv(EnvSignerPassword); signerPassword != "" {
		password = signerPassword
	} else {
		password, _ = input.GetPassword("Enter Signer password:", reader)
	}

	return strings.TrimSpace(username), strings.TrimSpace(password)
}

// GetKeybase will create an instance of Keybase
func GetKeybase(thorchainHome string) (KeybaseStore, error) {
	username, password := SignerCreds()
	buf := bytes.NewBufferString(password)
	// the library used by keyring is using ReadLine , which expect a new line
	buf.WriteByte('\n')

	cliDir := thorchainHome
	if len(thorchainHome) == 0 {
		usr, err := user.Current()
		if err != nil {
			return KeybaseStore{}, fmt.Errorf("fail to get current user,err:%w", err)
		}
		cliDir = filepath.Join(usr.HomeDir, ".thornode")
	}

	// Should we pass in the cdc?
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb, err := ckeys.New(KeyringServiceName(), ckeys.BackendFile, cliDir, buf, cdc, func(options *ckeys.Options) {
		options.SupportedAlgos = ckeys.SigningAlgoList{hd.Secp256k1, ed25519.Ed25519}
	})
	return KeybaseStore{
		SignerName:   username,
		SignerPasswd: password,
		Keybase:      kb,
	}, err
}

// SafeUintFromInt64 create a new Uint from an int64. It is expected that the int64 is
// positive - if not, we return zero to prevent overflow errors.
func SafeUintFromInt64(i int64) Uint {
	if i < 0 {
		return ZeroUint()
	}
	return NewUint(uint64(i))
}

// MinUint returns the minimum of two Uints.
func MinUint(u1, u2 Uint) Uint {
	if u1.LT(u2) {
		return u1
	}
	return u2
}

// Sum returns the sum of all the Uints in the slice.
func Sum(u []Uint) Uint {
	sum := ZeroUint()
	for _, i := range u {
		sum = sum.Add(i)
	}
	return sum
}
