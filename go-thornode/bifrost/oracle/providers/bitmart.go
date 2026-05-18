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

type BitmartTicker [13]string

type BitmartWsTickerMsg struct {
	Data []struct {
		Symbol      string `json:"symbol"`
		Price       string `json:"last_price"`
		BaseVolume  string `json:"base_volume_24h"`
		QuoteVolume string `json:"quote_volume_24h"`
	} `json:"data"`
}

type BitmartWsSubscriptionMsg struct {
	Topic string `json:"topic"`
	Event string `json:"event"`
}

type BitmartProvider struct {
	*websocketProvider
}

func NewBitmartProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*BitmartProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderBitmart, config, metrics)
	if err != nil {
		return nil, err
	}

	// https://developer-pro.bitmart.com/en/spot/#stay-connected-and-limit
	// client needs to send if there is no new message for 20s
	// default to ping every 15s
	wsProvider.keepaliveInterval = time.Second * 15
	wsProvider.keepaliveTimeout = time.Second * 30

	provider := BitmartProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *BitmartProvider) getTickers() ([]BitmartTicker, error) {
	data, err := p.httpGet("/spot/quotation/v3/tickers")
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []BitmartTicker `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (p *BitmartProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return nil, err
	}

	var symbols []string
	for _, ticker := range tickers {
		symbols = append(symbols, ticker[0])
	}

	return symbols, nil
}

func (p *BitmartProvider) ToProviderSymbol(base, quote string) string {
	return base + "_" + quote
}

// Polling

func (p *BitmartProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return
	}

	for _, ticker := range tickers {
		symbol := ticker[0]
		pair, found := p.pairs[symbol]
		if !found {
			continue
		}

		p.setTicker(
			time.Now(),
			pair,
			strToFloat(ticker[1]),
			strToFloat(ticker[2]),
			strToFloat(ticker[3]),
		)
	}
}

// Websocket

func (p *BitmartProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var args []string

	for _, pair := range p.sortedPairs() {
		args = append(args, "spot/ticker:"+p.ToProviderSymbol(pair.Base, pair.Quote))
	}

	msgs := []map[string]any{{
		"op":   "subscribe",
		"args": args,
	}}

	return msgs, nil
}

func (p *BitmartProvider) HandleWsMessage(msg []byte) error {
	var tickerMsg BitmartWsTickerMsg
	var subscriptionMsg BitmartWsSubscriptionMsg

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && len(tickerMsg.Data) > 0 && tickerMsg.Data[0].Symbol != "" {
		for _, ticker := range tickerMsg.Data {
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

		return nil
	}

	err = json.Unmarshal(msg, &subscriptionMsg)
	if err == nil && subscriptionMsg.Event == "subscribe" {
		return nil
	}

	if len(msg) == 4 && string(msg) == "pong" {
		p.lastKeepalive = time.Now()
		return nil
	}

	return ErrUnknownWsMsg
}

func (p *BitmartProvider) Ping() error {
	return p.ws.Write(p.ctx, websocket.MessageText, []byte("ping"))
}
