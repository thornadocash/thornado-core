package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestNewAnteHandlerNilAccountKeeper(t *testing.T) {
	_, err := NewAnteHandler(HandlerOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "account keeper is required")
}

func TestSigGasConsumerEthKey(t *testing.T) {
	ethKey := &ethsecp256k1.PubKey{Key: make([]byte, 33)}

	meter := storetypes.NewGasMeter(100000)
	sig := signing.SignatureV2{
		PubKey: ethKey,
	}
	params := authtypes.DefaultParams()

	err := SigGasConsumer(meter, sig, params)
	require.NoError(t, err)
	require.Greater(t, meter.GasConsumed(), uint64(0))
}

func TestSigGasConsumerSecp256k1Key(t *testing.T) {
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()

	meter := storetypes.NewGasMeter(100000)
	sig := signing.SignatureV2{
		PubKey: pubKey,
	}
	params := authtypes.DefaultParams()

	err := SigGasConsumer(meter, sig, params)
	require.NoError(t, err)
	require.Greater(t, meter.GasConsumed(), uint64(0))
}

func TestNewMsgServiceRouter(t *testing.T) {
	bAppMsr := baseapp.NewMsgServiceRouter()
	msr := NewMsgServiceRouter(bAppMsr)

	require.NotNil(t, msr)
	require.NotNil(t, msr.MsgServiceRouter)
	require.NotNil(t, msr.customRoutes)
}

func TestMsgServiceRouterAddCustomRoute(t *testing.T) {
	bAppMsr := baseapp.NewMsgServiceRouter()
	msr := NewMsgServiceRouter(bAppMsr)

	handler := struct{}{}
	msr.AddCustomRoute("test.service.v1", handler)

	require.NotNil(t, msr.customRoutes["test.service.v1"])
}

func TestRegisterSwaggerAPIDisabled(t *testing.T) {
	rtr := mux.NewRouter()

	err := RegisterSwaggerAPI(rtr, false)
	require.NoError(t, err)

	// ping endpoint should still be registered
	req := httptest.NewRequest(http.MethodGet, "/thornado/ping", nil)
	rr := httptest.NewRecorder()
	rtr.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "pong")

	for _, path := range []string{"/thornado", "/thornado/", "/thornado/ui/manifest.json"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		rtr.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "expected UI route %s when swagger disabled", path)
	}

	// swagger doc routes should NOT be registered when disabled
	for _, path := range []string{"/thornado/doc", "/thornado/doc/openapi.yaml", "/thornado/doc/openapi.json"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		rtr.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code, "expected 404 for %s when swagger disabled", path)
	}
}

func TestRegisterSwaggerAPIEnabled(t *testing.T) {
	rtr := mux.NewRouter()
	err := RegisterSwaggerAPI(rtr, true)
	require.NoError(t, err)

	// ping endpoint
	req := httptest.NewRequest(http.MethodGet, "/thornado/ping", nil)
	rr := httptest.NewRecorder()
	rtr.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	for _, path := range []string{"/thornado", "/thornado/", "/thornado/ui/manifest.json"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		rtr.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "expected UI route %s when swagger enabled", path)
	}

	// swagger doc routes should be registered when enabled
	for _, path := range []string{"/thornado/doc", "/thornado/doc/openapi.yaml", "/thornado/doc/openapi.json"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		rtr.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "expected 200 for %s when swagger enabled", path)
	}
}

func TestBlockedAddresses(t *testing.T) {
	addrs := BlockedAddresses()
	require.NotEmpty(t, addrs)

	// Lending and treasury should NOT be blocked
	lendingAddr := authtypes.NewModuleAddress("lending").String()
	treasuryAddr := authtypes.NewModuleAddress("treasury").String()
	require.False(t, addrs[lendingAddr])
	require.False(t, addrs[treasuryAddr])
}

func TestNewTestAppOptionsWithFlagHome(t *testing.T) {
	opts := NewTestAppOptionsWithFlagHome("/tmp/test")
	require.NotNil(t, opts)
}
