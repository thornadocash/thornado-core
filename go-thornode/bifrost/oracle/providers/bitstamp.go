package providers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type BitstampTicker struct {
	Symbol string `json:"pair"`
	Type   string `json:"market_type"`
	Price  string `json:"last"`
	Volume string `json:"volume"`
}

type BitstampProvider struct {
	*pollingProvider
}

func NewBitstampProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*BitstampProvider, error) {
	pollProvider, err := newPollingProvider(ctx, logger, common.ProviderBitstamp, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := BitstampProvider{pollProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *BitstampProvider) getTickers() ([]BitstampTicker, error) {
	data, err := p.httpGet("/api/v2/ticker/")
	if err != nil {
		p.logger.Err(err).Msg("error fetching data")
		return nil, err
	}

	var tickers []BitstampTicker

	err = json.Unmarshal(data, &tickers)
	if err != nil {
		return nil, err
	}

	return tickers, nil
}

func (p *BitstampProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("error fetching tickers")
		return nil, err
	}

	var symbols []string
	for _, ticker := range tickers {
		if ticker.Type != "SPOT" {
			continue
		}
		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *BitstampProvider) ToProviderSymbol(base, quote string) string {
	return base + "/" + quote
}

func (p *BitstampProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("error fetching tickers")
		return
	}

	now := time.Now()

	for _, ticker := range tickers {
		pair, found := p.pairs[ticker.Symbol]
		if !found {
			continue
		}

		p.setTicker(
			now,
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.Volume),
			nil,
		)
	}
}
