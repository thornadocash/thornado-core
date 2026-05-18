package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type CoinwTicker struct {
	Id     int64  `json:"id"`
	Symbol string `json:"symbol"`
	Price  string `json:"last"`
	// Volume string `json:"baseVolume"` returning quote volume atm
}

type CoinwWsSubscriptionMsg struct {
	Channel string `json:"channel"`
}

type CoinwWsTickerMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type CoinwWsTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"last"`
	Volume string `json:"vol"`
}

type CoinwWsPongMsg struct {
	Event string `json:"event"`
}

type CoinwProvider struct {
	*websocketProvider
}

func NewCoinwProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*CoinwProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderCoinw, config, metrics)
	if err != nil {
		return nil, err
	}

	wsProvider.keepaliveInterval = time.Second * 10
	wsProvider.keepaliveTimeout = time.Second * 20

	provider := CoinwProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *CoinwProvider) getTickers() ([]CoinwTicker, error) {
	data, err := p.httpGet("/api/v1/public?command=returnTicker")
	if err != nil {
		return nil, err
	}

	var response struct {
		Data map[string]CoinwTicker `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	var tickers []CoinwTicker

	for symbol, ticker := range response.Data {
		tickers = append(tickers, CoinwTicker{
			Id:     ticker.Id,
			Symbol: symbol,
			Price:  ticker.Price,
			// Volume: ticker.Volume,
		})
	}

	return tickers, nil
}

func (p *CoinwProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return nil, err
	}

	var symbols []string
	for _, item := range tickers {
		symbols = append(symbols, item.Symbol)
	}

	return symbols, nil
}

func (p *CoinwProvider) ToProviderSymbol(base, quote string) string {
	return base + "_" + quote
}

// Polling

func (p *CoinwProvider) Poll() {
	// can't be used currently as "baseVolume" returns the quote volume
}

// Websocket

func (p *CoinwProvider) PrepareConnection() error {
	// replace symbols by ticker ids, which is used in ws messages instead
	// e.g.: BTC_USDT -> 78

	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return err
	}

	for _, ticker := range tickers {
		pair, found := p.pairs[ticker.Symbol]
		if !found {
			continue
		}
		delete(p.pairs, ticker.Symbol)
		p.pairs[fmt.Sprintf("%d", ticker.Id)] = pair
	}

	return nil
}

func (p *CoinwProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	msgs := []map[string]any{}

	// sort for testing
	var ids []string
	for id := range p.pairs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		msgs = append(msgs, map[string]any{
			"event": "sub",
			"params": map[string]any{
				"biz":      "exchange",
				"type":     "ticker",
				"pairCode": id,
			},
		})
	}
	return msgs, nil
}

func (p *CoinwProvider) HandleWsMessage(msg []byte) error {
	var (
		tickerMsg       CoinwWsTickerMsg
		subscriptionMsg CoinwWsSubscriptionMsg
		pongMsg         CoinwWsPongMsg
	)

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && tickerMsg.Type == "ticker" && tickerMsg.Data != "" {
		var ticker CoinwWsTicker
		err = json.Unmarshal([]byte(tickerMsg.Data), &ticker)
		if err != nil {
			p.logger.Err(err).Msg("fail to parse ticker")
			return err
		}

		pair, found := p.pairs[ticker.Symbol]
		if !found {
			return nil
		}

		p.setTicker(
			time.Now(),
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.Volume),
			nil,
		)

		return nil
	}

	err = json.Unmarshal(msg, &subscriptionMsg)
	if err == nil && subscriptionMsg.Channel == "subscribe" {
		return nil
	}

	err = json.Unmarshal(msg, &pongMsg)
	if err == nil && pongMsg.Event == "pong" {
		p.lastKeepalive = time.Now()
		return nil
	}

	return ErrUnknownWsMsg
}

func (p *CoinwProvider) Ping() error {
	msg := map[string]any{
		"event": "ping",
	}

	return wsjson.Write(p.ctx, p.ws, msg)
}
