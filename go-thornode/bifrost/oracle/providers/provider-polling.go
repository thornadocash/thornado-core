package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/config"
)

type pollingProvider struct {
	*baseProvider
	poll func()
}

func newPollingProvider(
	ctx context.Context,
	logger zerolog.Logger,
	name string,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*pollingProvider, error) {
	if config.PollingInterval == 0 {
		return nil, fmt.Errorf("polling interval cannot be zero")
	}

	var err error

	provider := pollingProvider{}

	provider.baseProvider, err = newBaseProvider(ctx, logger, name, config, metrics)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *pollingProvider) init(provider types.PollingProvider) error {
	err := p.baseProvider.init(provider)
	if err != nil {
		p.logger.Err(err).Msg("Error initializing baseProvider")
		return err
	}

	p.poll = provider.Poll

	return nil
}

func (p *pollingProvider) Start() {
	p.logger.Info().Msg("starting provider")
	p.wg.Add(1)

	go func() {
		ticker := time.NewTicker(p.config.PollingInterval)
		defer ticker.Stop() // Always stop ticker to release resources

		for {
			select {
			case <-ticker.C:
				p.poll()
			case <-p.ctx.Done():
				return
			}
		}
	}()
}
