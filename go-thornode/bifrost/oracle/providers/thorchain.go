package providers

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type ThorchainProvider struct {
	*pollingProvider
}

func NewThorchainProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*ThorchainProvider, error) {
	pollProvider, err := newPollingProvider(ctx, logger, common.ProviderThorchain, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := ThorchainProvider{pollProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *ThorchainProvider) GetAvailablePairs() ([]string, error) {
	return []string{"RUNEUSD"}, nil
}

func (p *ThorchainProvider) ToProviderSymbol(base, quote string) string {
	return base + quote
}

func (p *ThorchainProvider) Poll() {
	data, err := p.httpGet("/thorchain/network")
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to get network data")
	}

	var status struct {
		RunePriceInTor string `json:"rune_price_in_tor"`
		TorPriceHalted bool   `json:"tor_price_halted"`
	}

	err = json.Unmarshal(data, &status)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to unmarshal status data")
		return
	}

	if status.TorPriceHalted {
		p.logger.Warn().Msg("Tor price halted")
		return
	}

	pair, err := types.NewCurrencyPair("RUNE/USD")
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create new pair")
		return
	}

	price := new(big.Float).Quo(strToFloat(status.RunePriceInTor), big.NewFloat(1e8))

	p.setTicker(
		time.Now(),
		pair,
		price,
		big.NewFloat(1),
		nil,
	)
}
