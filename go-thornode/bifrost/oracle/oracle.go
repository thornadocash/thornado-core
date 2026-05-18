package oracle

import (
	"context"
	"crypto/sha256"
	"math/big"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/providers"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type Oracle struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wg        *sync.WaitGroup
	mtx       *sync.Mutex
	logger    zerolog.Logger
	providers map[string]types.Provider
	config    config.Bifrost
	rates     map[string]*big.Float
	metrics   *metrics.Metrics
	symbols   []string
	version   []byte
}

func NewOracle(config config.Bifrost) (*Oracle, error) {
	logger := log.With().Str("module", "oracle").Logger()

	logLevel, err := zerolog.ParseLevel(config.Oracle.LogLevel)
	if err != nil {
		logger.Error().Err(err).Msg("invalid log level")
		logLevel = zerolog.InfoLevel
	}
	logger = logger.Level(logLevel)

	cv := constants.NewConstantValue()
	required := cv.GetStringValue(constants.RequiredPriceFeeds)

	hash := sha256.Sum256([]byte(required))

	ctx, cancel := context.WithCancel(context.Background())
	o := Oracle{
		ctx:     ctx,
		cancel:  cancel,
		mtx:     &sync.Mutex{},
		wg:      &sync.WaitGroup{},
		logger:  logger,
		config:  config,
		rates:   make(map[string]*big.Float),
		metrics: metrics.NewMetrics(config.Oracle.DetailedMetrics),
		symbols: strings.Split(required, ","),
		version: hash[:8],
	}

	o.metrics.Register()

	err = o.LoadProviders()
	if err != nil {
		return nil, err
	}

	return &o, nil
}

func (o *Oracle) Start() {
	if len(o.providers) == 0 {
		o.logger.Warn().Msg("no providers set")
		return
	}

	// set global price feed queue
	o.logger.Info().Msg("starting oracle")

	for name, provider := range o.providers {
		o.wg.Add(1)
		o.logger.Info().Str("provider", name).Msg("starting provider")
		go func() {
			provider.Start()
		}()
	}
}

func (o *Oracle) Stop() {
	o.logger.Info().Msg("stopping oracle")
	o.cancel()
}

func (o *Oracle) GetPrices() ([]*common.OraclePrice, []byte) {
	if len(o.providers) == 0 {
		return nil, nil
	}

	tickerBySymbol := map[string]map[string]types.Ticker{}

	for name, provider := range o.providers {
		tickers, err := provider.GetTickers()
		if err != nil {
			o.logger.Err(err).
				Str("provider", name).
				Msg("failed to get provider prices")
		}

		for symbol, ticker := range tickers {
			_, found := tickerBySymbol[symbol]
			if !found {
				tickerBySymbol[symbol] = map[string]types.Ticker{}
			}
			tickerBySymbol[symbol][name] = ticker
		}
	}

	// actually compute the final prices from all providers
	converter := NewConverter(tickerBySymbol, o.metrics)
	rates, err := converter.ConvertToUsd()
	if err != nil {
		o.logger.Err(err).Msg("failed to convert to USD")
		return nil, nil
	}

	prices := make([]*common.OraclePrice, len(o.symbols))

	for i, symbol := range o.symbols {
		rate, found := rates[symbol]
		if !found {
			rate = &big.Float{}
		}
		price, err := common.NewOraclePrice(rate)
		if err != nil {
			o.logger.Err(err).Msg("fail creating oracle price")
			continue
		}
		prices[i] = price
	}

	return prices, o.version
}

func (o *Oracle) LoadProviders() error {
	if !o.config.Oracle.Enabled {
		o.logger.Info().Msg("oracle disabled")
		return nil
	}

	o.logger.Info().Msg("loading providers")
	o.providers = make(map[string]types.Provider)

	for providerName, providerConfig := range o.config.GetProviders() {
		var provider types.Provider
		var err error

		if providerConfig.Disabled {
			o.logger.Info().
				Str("provider", providerName).
				Msg("provider disabled")
			continue
		}

		switch providerName {
		case common.ProviderBinance:
			provider, err = providers.NewBinanceProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderBitfinex:
			provider, err = providers.NewBitfinexProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderBitget:
			provider, err = providers.NewBitgetProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderBitmart:
			provider, err = providers.NewBitmartProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderBitstamp:
			provider, err = providers.NewBitstampProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderBybit:
			provider, err = providers.NewBybitProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderCrypto:
			provider, err = providers.NewCryptoProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderCoinbase:
			provider, err = providers.NewCoinbaseProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderCoinw:
			provider, err = providers.NewCoinwProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderDigifinex:
			provider, err = providers.NewDigifinexProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderGate:
			provider, err = providers.NewGateProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderGemini:
			provider, err = providers.NewGeminiProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderHtx:
			provider, err = providers.NewHtxProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderKraken:
			provider, err = providers.NewKrakenProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderKucoin:
			provider, err = providers.NewKucoinProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderLbank:
			provider, err = providers.NewLbankProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderMexc:
			provider, err = providers.NewMexcProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderOkx:
			provider, err = providers.NewOkxProvider(o.ctx, o.logger, providerConfig, o.metrics)
		case common.ProviderThorchain:
			provider, err = providers.NewThorchainProvider(o.ctx, o.logger, providerConfig, o.metrics)
		default:
			o.logger.Error().Msgf("unknown provider: %s", providerName)
			continue
		}

		if err != nil {
			o.logger.Err(err).Msgf("error loading provider: %s", providerName)
			continue
		}

		o.providers[providerName] = provider
	}

	if len(o.providers) == 0 {
		o.logger.Warn().Msg("no providers loaded")
	}

	return nil
}
