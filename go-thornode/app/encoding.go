package app

import (
	"os"

	"cosmossdk.io/log"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/flags"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"

	"gitlab.com/thorchain/thornode/v3/app/params"
)

const (
	TestApp = "testApp"
)

// MakeEncodingConfig creates a new EncodingConfig with all modules registered. For testing only
func MakeEncodingConfig() params.EncodingConfig {
	dir, err := os.MkdirTemp("", "temp_bifrost")
	if err != nil {
		panic("failed to create temp_bifrost dir: " + err.Error())
	}
	defer os.RemoveAll(dir)
	// we "pre"-instantiate the application for getting the injected/configured encoding configuration
	// note, this is not necessary when using app wiring, as depinject can be directly used (see root_v2.go)
	tempApp := NewChainApp(
		log.NewNopLogger(),
		dbm.NewMemDB(),
		nil,
		false,
		NewTestAppOptionsWithFlagHome(dir),
		[]wasmkeeper.Option{},
	)
	return makeEncodingConfig(tempApp)
}

func makeEncodingConfig(tempApp *THORChainApp) params.EncodingConfig {
	encodingConfig := params.EncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}
	return encodingConfig
}

func NewTestAppOptionsWithFlagHome(homePath string) servertypes.AppOptions {
	return simtestutil.AppOptionsMap{
		flags.FlagHome: homePath,
		TestApp:        true,
	}
}
