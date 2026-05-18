package providers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type BitgetTicker struct {
	Symbol      string `json:"symbol"`
	Price       string `json:"lastPr"`
	BaseVolume  string `json:"baseVolume"`
	QuoteVolume string `json:"quoteVolume"`
}

type BitgetWsTickerMsg struct {
	Data []struct {
		Symbol      string `json:"instId"`
		Price       string `json:"lastPr"`
		BaseVolume  string `json:"baseVolume"`
		QuoteVolume string `json:"quoteVolume"`
	} `json:"data"`
}

type BitgetWsEventMsg struct {
	Event string            `json:"event"`
	Arg   map[string]string `json:"arg"`
}

type BitgetProvider struct {
	*websocketProvider
}

func NewBitgetProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*BitgetProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderBitget, config, metrics)
	if err != nil {
		return nil, err
	}

	// https://www.bitget.com/api-doc/common/websocket-intro
	// client needs to send a ping every 30s and expect a pong response
	wsProvider.keepaliveInterval = time.Second * 30
	wsProvider.keepaliveTimeout = time.Second * 35

	provider := BitgetProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *BitgetProvider) getTickers() ([]BitgetTicker, error) {
	data, err := p.httpGet("/api/v2/spot/market/tickers")
	if err != nil {
		p.logger.Err(err).Msg("error fetching data")
		return nil, err
	}

	var response struct {
		Data []BitgetTicker `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (p *BitgetProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("error fetching tickers")
		return nil, err
	}

	var symbols []string
	for _, ticker := range tickers {
		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *BitgetProvider) ToProviderSymbol(base, quote string) string {
	return base + quote
}

func (p *BitgetProvider) Poll() {
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
			strToFloat(ticker.BaseVolume),
			strToFloat(ticker.QuoteVolume),
		)
	}
}

// Websocket

func (p *BitgetProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var args []map[string]string

	for _, pair := range p.sortedPairs() {
		args = append(args, map[string]string{
			"instType": "SPOT",
			"channel":  "ticker",
			"instId":   pair.Join(""),
		})
	}

	msgs := []map[string]any{{
		"op":   "subscribe",
		"args": args,
	}}

	return msgs, nil
}

func (p *BitgetProvider) HandleWsMessage(msg []byte) error {
	if len(msg) == 0 {
		return nil
	}

	var tickerMsg BitgetWsTickerMsg
	var eventMsg BitgetWsEventMsg

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && len(tickerMsg.Data) > 0 {
		now := time.Now()

		for _, ticker := range tickerMsg.Data {
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

		return nil
	}

	err = json.Unmarshal(msg, &eventMsg)
	if err == nil && eventMsg.Event == "subscribe" {
		// subscription response
		return nil
	}

	if len(msg) == 4 {
		if string(msg) == "pong" {
			p.lastKeepalive = time.Now()
			return nil
		}
	}

	return ErrUnknownWsMsg
}

func (p *BitgetProvider) Ping() error {
	return p.ws.Write(p.ctx, websocket.MessageText, []byte("ping"))
}
