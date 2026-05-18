package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type GeminiTicker struct {
	Price  string         `json:"last"`
	Volume map[string]any `json:"volume"`
}

type GeminiProvider struct {
	*pollingProvider
}

func NewGeminiProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*GeminiProvider, error) {
	pollProvider, err := newPollingProvider(ctx, logger, common.ProviderGemini, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := GeminiProvider{pollProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *GeminiProvider) GetAvailablePairs() ([]string, error) {
	data, err := p.httpGet("/v1/symbols")
	if err != nil {
		p.logger.Err(err).Msg("error fetching symbols")
		return nil, err
	}

	var symbols []string
	err = json.Unmarshal(data, &symbols)
	if err != nil {
		p.logger.Err(err).Msg("fail to unmarshal data")
		return nil, err
	}

	return symbols, nil
}

func (p *GeminiProvider) ToProviderSymbol(base, quote string) string {
	return strings.ToLower(base + quote)
}

func (p *GeminiProvider) Poll() {
	i := 0
	for symbol, pair := range p.pairs {
		go func(p *GeminiProvider, symbol string, pair types.CurrencyPair) {
			path := fmt.Sprintf("/v1/pubticker/%s", symbol)
			data, err := p.httpGet(path)
			if err != nil {
				p.logger.Err(err).Msg("error fetching ticker")
				return
			}

			var ticker GeminiTicker
			err = json.Unmarshal(data, &ticker)
			if err != nil {
				p.logger.Err(err).Msg("error parsing ticker")
				return
			}

			value, found := ticker.Volume[pair.Base]
			if !found {
				p.logger.Error().Msg("no volume found")
				return
			}

			volume, ok := value.(string)
			if !ok {
				p.logger.Error().Msg("error parsing volume")
				return
			}

			now := time.Now()

			p.setTicker(
				now,
				pair,
				strToFloat(ticker.Price),
				strToFloat(volume),
				nil,
			)
		}(p, symbol, pair)
		// Gemini has a rate limit of 2req/s, sleeping 1.2s before running
		// the next batch of requests
		i++
		if i == 2 {
			i = 0
			time.Sleep(time.Millisecond * 1200)
		}
	}
}
