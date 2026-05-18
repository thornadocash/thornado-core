package providers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type BybitTickersResponse struct {
	Result struct {
		List []struct {
			Symbol      string `json:"symbol"`
			Price       string `json:"lastPrice"`
			BaseVolume  string `json:"volume24h"`
			QuoteVolume string `json:"turnover24h"`
		} `json:"list"`
	} `json:"result"`
}

type BybitWsTickerMsg struct {
	Data struct {
		Symbol      string `json:"symbol"`
		Price       string `json:"lastPrice"`
		BaseVolume  string `json:"volume24h"`
		QuoteVolume string `json:"turnover24h"`
	} `json:"data"`
}

type BybitWsPongMsg struct {
	RetMsg string `json:"ret_msg"`
	Op     string `json:"op"`
}

type BybitWsSubscriptionMsg struct {
	RetMsg string `json:"ret_msg"`
}

type BybitProvider struct {
	*websocketProvider
}

func NewBybitProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*BybitProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderBybit, config, metrics)
	if err != nil {
		return nil, err
	}

	// https://bybit-exchange.github.io/docs/v5/ws/connect
	// client needs to send a ping every few seconds using 20s as interval
	wsProvider.keepaliveInterval = time.Second * 20
	wsProvider.keepaliveTimeout = time.Second * 30

	provider := BybitProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *BybitProvider) Poll() {
	data, err := p.httpGet("/v5/market/tickers?category=spot")
	if err != nil {
		p.logger.Err(err).Msg("error fetching data")
		return
	}

	var response BybitTickersResponse

	err = json.Unmarshal(data, &response)
	if err != nil {
		p.logger.Err(err).Msg("error unmarshalling data")
		return
	}

	now := time.Now()

	for _, ticker := range response.Result.List {
		pair, found := p.pairs[ticker.Symbol]
		if !found {
			continue
		}

		p.setTicker(
			now,
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.BaseVolume),
			strToFloat(ticker.QuoteVolume),
		)
	}
}

func (p *BybitProvider) GetAvailablePairs() ([]string, error) {
	data, err := p.httpGet("/v5/market/instruments-info?category=spot")
	if err != nil {
		return nil, err
	}

	var response struct {
		Result struct {
			List []struct {
				Symbol string `json:"symbol"`
			} `json:"list"`
		} `json:"result"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, item := range response.Result.List {
		symbols = append(symbols, item.Symbol)
	}

	return symbols, nil
}

func (p *BybitProvider) ToProviderSymbol(base, quote string) string {
	return base + quote
}

func (p *BybitProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	msgs := []map[string]any{}

	var args []string
	var count int

	for _, pair := range p.sortedPairs() {
		count++
		args = append(args, "tickers."+p.toMappedProviderSymbol(pair))

		if len(args) >= 5 || count == len(p.pairs) {
			msgs = append(msgs, map[string]any{
				"op":   "subscribe",
				"args": args,
			})

			args = []string{}
		}
	}

	return msgs, nil
}

func (p *BybitProvider) HandleWsMessage(msg []byte) error {
	var (
		tickerMsg       BybitWsTickerMsg
		subscriptionMsg BybitWsSubscriptionMsg
		pongMsg         BybitWsPongMsg
	)

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && tickerMsg.Data.Symbol != "" {

		pair, found := p.pairs[tickerMsg.Data.Symbol]
		if !found {
			return nil
		}

		timestamp := time.Now()

		p.setTicker(
			timestamp,
			pair,
			strToFloat(tickerMsg.Data.Price),
			strToFloat(tickerMsg.Data.BaseVolume),
			strToFloat(tickerMsg.Data.QuoteVolume),
		)

		return nil
	}

	err = json.Unmarshal(msg, &pongMsg)
	if err == nil && pongMsg.RetMsg == "pong" && pongMsg.Op == "ping" {
		p.lastKeepalive = time.Now()
		return nil
	}

	err = json.Unmarshal(msg, &subscriptionMsg)
	if err == nil && subscriptionMsg.RetMsg == "subscribe" {
		return nil
	}

	return ErrUnknownWsMsg
}

func (p *BybitProvider) Ping() error {
	msg := map[string]any{
		"op": "ping",
	}

	return wsjson.Write(p.ctx, p.ws, msg)
}
