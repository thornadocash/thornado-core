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

type MexcTicker struct {
	Symbol      string `json:"symbol"`
	Price       string `json:"lastPrice"`
	BaseVolume  string `json:"volume"`
	QuoteVolume string `json:"quoteVolume"`
}

type MexcProvider struct {
	*pollingProvider
}

func NewMexcProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*MexcProvider, error) {
	pollProvider, err := newPollingProvider(ctx, logger, common.ProviderMexc, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := MexcProvider{pollProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *MexcProvider) getTickers() ([]MexcTicker, error) {
	data, err := p.httpGet("/api/v3/ticker/24hr")
	if err != nil {
		return nil, err
	}

	var tickers []MexcTicker
	err = json.Unmarshal(data, &tickers)
	if err != nil {
		return nil, err
	}

	return tickers, nil
}

func (p *MexcProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, ticker := range tickers {
		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *MexcProvider) ToProviderSymbol(base, quote string) string {
	return base + quote
}

func (p *MexcProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return
	}

	for _, ticker := range tickers {
		pair, found := p.pairs[ticker.Symbol]
		if !found {
			continue
		}

		p.setTicker(
			time.Now(),
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.BaseVolume),
			strToFloat(ticker.QuoteVolume),
		)
	}
}
