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

type GateTicker struct {
	Symbol      string `json:"currency_pair"`
	Price       string `json:"last"`
	BaseVolume  string `json:"base_volume"`
	QuoteVolume string `json:"quote_volume"`
}

type GateWsTickerMsg struct {
	Result GateTicker `json:"result"`
}

type GateProvider struct {
	*websocketProvider
}

func NewGateProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*GateProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderGate, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := GateProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *GateProvider) getTickers() ([]GateTicker, error) {
	data, err := p.httpGet("/api/v4/spot/tickers")
	if err != nil {
		return nil, err
	}

	var tickers []GateTicker
	err = json.Unmarshal(data, &tickers)
	if err != nil {
		return nil, err
	}

	return tickers, nil
}

func (p *GateProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		return nil, err
	}

	symbols := []string{}
	for _, ticker := range tickers {
		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *GateProvider) ToProviderSymbol(base, quote string) string {
	return base + "_" + quote
}

func (p *GateProvider) Poll() {
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

func (p *GateProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var symbols []string

	for _, pair := range p.sortedPairs() {
		symbols = append(symbols, p.toMappedProviderSymbol(pair))
	}

	msgs := []map[string]any{{
		"time":    time.Now().Unix(),
		"channel": "spot.tickers",
		"event":   "subscribe",
		"payload": symbols,
	}}

	return msgs, nil
}

func (p *GateProvider) HandleWsMessage(msg []byte) error {
	var tickerMsg GateWsTickerMsg
	err := json.Unmarshal(msg, &tickerMsg)
	if err != nil {
		p.logger.Err(err).Msg("error unmarshalling msg")
		return err
	}

	ticker := tickerMsg.Result

	pair, found := p.pairs[ticker.Symbol]
	if !found {
		return nil
	}

	timestamp := time.Now()

	p.setTicker(
		timestamp,
		pair,
		strToFloat(ticker.Price),
		strToFloat(ticker.BaseVolume),
		strToFloat(ticker.QuoteVolume),
	)

	return nil
}
