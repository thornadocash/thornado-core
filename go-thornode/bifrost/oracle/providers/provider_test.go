package providers

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/providers/mock"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"

	. "gopkg.in/check.v1"
)

type ProviderTestSuite struct {
	server  *httptest.Server
	wsUrl   string
	ctx     context.Context
	logger  zerolog.Logger
	config  map[string]config.BifrostOracleProviderConfiguration
	metrics *metrics.Metrics
}

var _ = Suite(&ProviderTestSuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *ProviderTestSuite) SetUpSuite(_ *C) {
	symbols := []string{
		"BTC/USDT",
		"ETH/USDT",
		"BTC/USD",
		"ETH/USD",
	}

	s.server = mock.NewServer()
	s.wsUrl = strings.Replace(s.server.URL, "http", "ws", 1)
	s.ctx = context.Background()
	s.logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	s.config = map[string]config.BifrostOracleProviderConfiguration{}
	for _, provider := range []string{
		common.ProviderBinance,
		common.ProviderBitfinex,
		common.ProviderBitget,
		common.ProviderBitmart,
		common.ProviderBitstamp,
		common.ProviderBybit,
		common.ProviderCoinbase,
		common.ProviderCoinw,
		common.ProviderCrypto,
		common.ProviderDigifinex,
		common.ProviderGate,
		common.ProviderGemini,
		common.ProviderHtx,
		common.ProviderKraken,
		common.ProviderKucoin,
		common.ProviderLbank,
		common.ProviderMexc,
		common.ProviderOkx,
		common.ProviderThorchain,
	} {
		s.config[provider] = s.newProviderConfig(provider, symbols)
	}
	s.metrics = metrics.NewMetrics(false)
}

func (s *ProviderTestSuite) TearDownSuite(_ *C) {
	s.server.Close()
}

func (s *ProviderTestSuite) newProviderConfig(
	name string,
	symbols []string,
) config.BifrostOracleProviderConfiguration {
	return config.BifrostOracleProviderConfiguration{
		Name:            name,
		Disabled:        false,
		PollingInterval: time.Second,
		ApiEndpoints:    []string{fmt.Sprintf("%s/%s", s.server.URL, name)},
		WsEndpoints:     []string{fmt.Sprintf("%s/%s/ws", s.wsUrl, name)},
		Pairs:           symbols,
	}
}

func newPair(symbol string) types.CurrencyPair {
	pair, err := types.NewCurrencyPair(symbol)
	if err != nil {
		return types.CurrencyPair{}
	}
	return pair
}
