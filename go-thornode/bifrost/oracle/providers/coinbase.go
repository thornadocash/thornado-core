package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type CoinbaseTicker struct {
	Price  string `json:"price"`
	Volume string `json:"volume"`
	Time   string `json:"time"`
}

type CoinbaseWsTickerMsg struct {
	Symbol string `json:"product_id"`
	Price  string `json:"price"`
	Volume string `json:"volume_24h"`
	Time   string `json:"time"`
}

type CoinbaseProvider struct {
	*websocketProvider
}

func NewCoinbaseProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*CoinbaseProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderCoinbase, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := CoinbaseProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *CoinbaseProvider) Poll() {
	i := 0
	for symbol, pair := range p.pairs {
		go func(p *CoinbaseProvider, symbol string, pair types.CurrencyPair) {
			path := fmt.Sprintf("/products/%s/ticker", symbol)
			data, err := p.httpGet(path)
			if err != nil {
				return
			}

			var ticker CoinbaseTicker
			err = json.Unmarshal(data, &ticker)
			if err != nil {
				return
			}

			now := time.Now()

			p.setTicker(
				now,
				pair,
				strToFloat(ticker.Price),
				strToFloat(ticker.Volume),
				nil,
			)
		}(p, symbol, pair)
		// Coinbase has a rate limit of 10req/s, sleeping 1.2s before running
		// the next batch of requests
		i++
		if i == 10 {
			i = 0
			time.Sleep(time.Millisecond * 1200)
		}
	}
}

func (p *CoinbaseProvider) ToProviderSymbol(base, quote string) string {
	return base + "-" + quote
}

// Polling

func (p *CoinbaseProvider) GetAvailablePairs() ([]string, error) {
	data, err := p.httpGet("/products")
	if err != nil {
		return nil, err
	}

	var response []struct {
		Id     string `json:"id"`
		Status string `json:"status"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, product := range response {
		if product.Status != "online" {
			continue
		}
		symbols = append(symbols, product.Id)
	}

	return symbols, nil
}

// Websocket

func (p *CoinbaseProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var symbols []string

	for _, pair := range p.sortedPairs() {
		symbols = append(symbols, p.toMappedProviderSymbol(pair))
	}

	msgs := []map[string]any{{
		"type":        "subscribe",
		"product_ids": symbols,
		"channels":    []string{"ticker"},
	}}

	return msgs, nil
}

func (p *CoinbaseProvider) HandleWsMessage(msg []byte) error {
	var tickerMsg CoinbaseWsTickerMsg
	err := json.Unmarshal(msg, &tickerMsg)
	if err != nil {
		p.logger.Err(err).Msg("error unmarshalling msg")
		return err
	}

	pair, found := p.pairs[tickerMsg.Symbol]
	if !found {
		return nil
	}

	timestamp := time.Now()

	p.setTicker(
		timestamp,
		pair,
		strToFloat(tickerMsg.Price),
		strToFloat(tickerMsg.Volume),
		nil,
	)

	return nil
}
