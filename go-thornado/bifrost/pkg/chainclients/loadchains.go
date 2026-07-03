package chainclients

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
)

// LoadChains returns chain clients from chain configuration
func LoadChains(thorKeys *thornadoclient.Keys,
	cfg map[common.Chain]config.BifrostChainConfiguration,
	thornadoBridge thornadoclient.ThornadoBridge,
	localState storage.LocalStateManager,
	m *metrics.Metrics,
	pubKeyValidator pubkeymanager.PubKeyValidator,
	coordinator frost.SessionCoordinator,
) (chains map[common.Chain]*btc.Client, restart chan struct{}) {
	logger := log.Logger.With().Str("module", "bifrost").Logger()

	chains = make(map[common.Chain]*btc.Client)
	restart = make(chan struct{})
	failedChains := []common.Chain{}

	loadChain := func(chain config.BifrostChainConfiguration) (*btc.Client, error) {
		if chain.ChainID != common.BTCChain {
			return nil, fmt.Errorf("chain %s is not supported by thornado bifrost", chain.ChainID)
		}
		return btc.NewClient(thorKeys, chain, thornadoBridge, localState, m, coordinator)
	}

	for _, chain := range cfg {
		if chain.Disabled {
			logger.Info().Msgf("%s chain is disabled by configure", chain.ChainID)
			continue
		}

		client, err := loadChain(chain)
		if err != nil {
			logger.Error().Err(err).Stringer("chain", chain.ChainID).Msg("failed to load chain")
			failedChains = append(failedChains, chain.ChainID)
			continue
		}

		pubKeyValidator.RegisterCallback(client.RegisterPublicKey)
		pubKeyValidator.RegisterPathCallback(client.RegisterPublicKeyAtPath)
		chains[chain.ChainID] = client
	}

	// watch failed chains minutely and restart bifrost if any succeed init
	if len(failedChains) > 0 {
		go func() {
			tick := time.NewTicker(time.Minute)
			for range tick.C {
				for _, chain := range failedChains {
					ccfg := cfg[chain]
					ccfg.BlockScanner.DBPath = "" // in-memory db

					_, err := loadChain(ccfg)
					if err == nil {
						logger.Info().Stringer("chain", chain).Msg("chain loaded, restarting bifrost")
						close(restart)
						return
					} else {
						logger.Error().Err(err).Stringer("chain", chain).Msg("failed to load chain")
					}
				}
			}
		}()
	}

	return chains, restart
}
