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

type KrakenTickersResponse struct {
	Result map[string]struct {
		Price  [2]string `json:"c"` // ex.: ["0.52900","94.23583387"]
		Volume [2]string `json:"v"` // ex.: ["6512.53593495","9341.68221855"]
	} `json:"result"`
}

type KrakenWsGenericMsg struct {
	Channel string `json:"channel"`
}

type KrakenWsSubscriptionMsg struct {
	Method string `json:"method"`
}

type KrakenWsTickerMsg struct {
	Channel string `json:"channel"`
	Data    []struct {
		Symbol string  `json:"symbol"`
		Price  float64 `json:"last"`
		Volume float64 `json:"volume"`
	} `json:"data"`
}

type KrakenProvider struct {
	*websocketProvider
}

func NewKrakenProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*KrakenProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderKraken, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := KrakenProvider{wsProvider}

	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *KrakenProvider) Poll() {
	data, err := p.httpGet("/0/public/Ticker")
	if err != nil {
		p.logger.Err(err).Msg("error fetching data")
		return
	}

	var response KrakenTickersResponse

	err = json.Unmarshal(data, &response)
	if err != nil {
		p.logger.Err(err).Msg("error unmarshalling data")
		return
	}

	now := time.Now()

	for symbol, ticker := range response.Result {
		pair, found := p.pairs[symbol]
		if !found {
			continue
		}

		p.setTicker(
			now,
			pair,
			strToFloat(ticker.Price[0]),
			strToFloat(ticker.Volume[1]),
			nil,
		)
	}
}

func (p *KrakenProvider) GetAvailablePairs() ([]string, error) {
	data, err := p.httpGet("/0/public/AssetPairs")
	if err != nil {
		return nil, err
	}

	var response struct {
		Result map[string]struct {
			Status string `json:"status"`
		} `json:"result"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for symbol, item := range response.Result {
		if item.Status != "online" {
			continue
		}
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

func (p *KrakenProvider) ToProviderSymbol(base, quote string) string {
	return base + quote
}

// Websocket

func (p *KrakenProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var symbols []string

	for _, pair := range p.sortedPairs() {
		symbols = append(symbols, pair.String())
	}

	msgs := []map[string]any{{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "ticker",
			"symbol":  symbols,
		},
	}}

	return msgs, nil
}

func (p *KrakenProvider) HandleWsMessage(msg []byte) error {
	var (
		tickerMsg       KrakenWsTickerMsg
		subscriptionMsg KrakenWsSubscriptionMsg
		genericMsg      KrakenWsGenericMsg
	)

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && tickerMsg.Channel == "ticker" && len(tickerMsg.Data) > 0 {
		if len(tickerMsg.Data) != 1 {
			return ErrUnknownWsMsg
		}

		ticker := tickerMsg.Data[0]

		var pair types.CurrencyPair
		pair, err = types.NewCurrencyPair(ticker.Symbol)
		if err != nil {
			p.logger.Err(err).Msg("fail to create pair")
			return err
		}

		symbol := p.toMappedProviderSymbol(pair)

		_, found := p.pairs[symbol]
		if !found {
			return nil
		}

		timestamp := time.Now()

		p.setTicker(
			timestamp,
			pair,
			big.NewFloat(ticker.Price),
			big.NewFloat(ticker.Volume),
			nil,
		)

		return nil
	}

	err = json.Unmarshal(msg, &subscriptionMsg)
	if err == nil && subscriptionMsg.Method == "subscribe" {
		return nil
	}

	err = json.Unmarshal(msg, &genericMsg)
	if err == nil && genericMsg.Channel != "" {
		switch genericMsg.Channel {
		case "status", "heartbeat":
			return nil
		}
	}

	return ErrUnknownWsMsg
}
