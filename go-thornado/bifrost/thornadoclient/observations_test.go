package thornadoclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/x/thornado"
	stypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestGetInboundOutboundAcceptsBTCDepositChildAddress(t *testing.T) {
	thornado.SetupConfigForTest()

	pubKey := stypes.GetRandomPubKey()
	baseAddress, err := pubKey.GetAddress(common.BTCChain)
	require.NoError(t, err)
	depositPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	require.NoError(t, err)
	depositAddress, err := common.DeriveBTCTaprootAddress(pubKey, depositPath)
	require.NoError(t, err)
	sender, err := common.NewAddress("bcrt1qejdlz8pqe8g7908j69mr6crlgfve67nm9u2hhr")
	require.NoError(t, err)

	bridge := thornadoBridge{logger: zerolog.Nop()}
	inbound, outbound, err := bridge.GetInboundOutbound(common.ObservedTxs{
		{
			Tx: common.Tx{
				ID:          "deposit",
				Chain:       common.BTCChain,
				FromAddress: sender,
				ToAddress:   depositAddress,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
		{
			Tx: common.Tx{
				ID:          "outbound",
				Chain:       common.BTCChain,
				FromAddress: depositAddress,
				ToAddress:   sender,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
		{
			Tx: common.Tx{
				ID:          "base",
				Chain:       common.BTCChain,
				FromAddress: sender,
				ToAddress:   baseAddress,
				Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
			},
			ObservedPubKey: pubKey,
		},
	})

	require.NoError(t, err)
	require.Len(t, inbound, 2)
	require.Len(t, outbound, 1)
	require.Equal(t, common.TxID("deposit"), inbound[0].Tx.ID)
	require.Equal(t, common.TxID("base"), inbound[1].Tx.ID)
	require.Equal(t, common.TxID("outbound"), outbound[0].Tx.ID)
}

func TestIsVaultDepositAddressAcceptsStringPathIndex(t *testing.T) {
	thornado.SetupConfigForTest()

	address, err := common.NewAddress("bcrt1p2etzngnwemdt3sandgxvlh8xm5ghm2ea2hd7c30wms3dru9fq7cq06nne3")
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/thornado/deposit/address/"+address.String())
		_, _ = w.Write([]byte(`{"address":"` + address.String() + `","vault_pub_key":"tthorpub1addwnpepqwc3qf8t0u7fat5r8w56jt9vm9tkhj5a8nldc9fv8r92hqcw86gvuzrdwjw","path_index":"1"}`))
	}))
	defer server.Close()

	client := retryablehttp.NewClient()
	client.RetryMax = 0
	bridge := thornadoBridge{
		logger:     zerolog.Nop(),
		cfg:        config.BifrostClientConfiguration{ChainHost: server.URL},
		httpClient: client,
		errCounter: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_errors_total"}, []string{"error", "endpoint"}),
	}

	require.True(t, bridge.IsVaultDepositAddress(address))
}
