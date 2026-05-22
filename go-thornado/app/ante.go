package app

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/evm/crypto/ethsecp256k1"

	storetypes "cosmossdk.io/store/types"

	"github.com/thornadocash/go-thornado/x/thorchain"
	"github.com/thornadocash/go-thornado/x/thorchain/ebifrost"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

// HandlerOptions extend the SDK's AnteHandler options
type HandlerOptions struct {
	ante.HandlerOptions

	BypassMinFeeMsgTypes []string

	ThornadoKeeper keeper.Keeper
}

// NewAnteHandler constructor
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, errors.New("account keeper is required for ante builder")
	}
	if options.BankKeeper == nil {
		return nil, errors.New("bank keeper is required for ante builder")
	}
	if options.SignModeHandler == nil {
		return nil, errors.New("sign mode handler is required for ante builder")
	}
	if options.ThornadoKeeper == nil {
		return nil, errors.New("thornado keeper is required for ante builder")
	}

	anteDecorators := []sdk.AnteDecorator{
		// must be first to ensure that injected txs bypass the remaining ante handlers, as they do not have gas.
		ebifrost.NewInjectedTxDecorator(),

		ante.NewSetUpContextDecorator(), // outermost AnteDecorator. SetUpContext must be called first

		// replace gas meter immediately after setting up ctx
		thorchain.NewGasDecorator(options.ThornadoKeeper),

		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),

		// run thornado-specific msg antes
		thorchain.NewAnteDecorator(options.ThornadoKeeper),

		ante.NewSetPubKeyDecorator(options.AccountKeeper), // SetPubKeyDecorator must be called before all signature verification decorators
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
	}

	return sdk.ChainAnteDecorators(anteDecorators...), nil
}

func SigGasConsumer(
	meter storetypes.GasMeter, sig signing.SignatureV2, params authtypes.Params,
) error {
	pubkey := sig.PubKey
	switch pubkey.(type) {
	case *ethsecp256k1.PubKey:
		// Ethereum keys
		meter.ConsumeGas(params.SigVerifyCostSecp256k1, "ante verify: eth_secp256k1")
		return nil
	default:
		return ante.DefaultSigVerificationGasConsumer(meter, sig, params)
	}
}
