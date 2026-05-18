package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type KucoinTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"last"`
	Volume string `json:"vol"`
}

type KucoinWsMarketMsg struct {
	Data struct {
		Data struct {
			Symbol string  `json:"symbol"`
			Price  float64 `json:"lastTradedPrice"`
			Volume float64 `json:"vol"`
		} `json:"data"`
	} `json:"data"`
}

type KucoinWsPongMsg struct {
	Id   string `json:"id"`
	Type string `json:"type"`
}

type KucoinWsGenericMsg struct {
	Type string `json:"type"`
}

type KucoinProvider struct {
	*websocketProvider
}

func NewKucoinProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*KucoinProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderKucoin, config, metrics)
	if err != nil {
		return nil, err
	}

	// https://www.kucoin.com/docs-new/websocket-api/base-info/introduction#4-ping
	// client needs to send a ping every few seconds returned by /api/v1/bullet-public
	// using 10s as default
	wsProvider.keepaliveInterval = time.Second * 10
	wsProvider.keepaliveTimeout = time.Second * 20

	provider := KucoinProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *KucoinProvider) getTickers() ([]KucoinTicker, error) {
	data, err := p.httpGet("/api/v1/market/allTickers")
	if err != nil {
		return nil, err
	}

	var response struct {
		Data struct {
			Ticker []KucoinTicker `json:"ticker"`
		} `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data.Ticker, nil
}

func (p *KucoinProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, item := range tickers {
		symbols = append(symbols, item.Symbol)
	}

	return symbols, nil
}

func (p *KucoinProvider) ToProviderSymbol(base, quote string) string {
	return base + "-" + quote
}

// Polling

func (p *KucoinProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
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
			strToFloat(ticker.Volume),
			nil,
		)
	}
}

// Websocket

func (p *KucoinProvider) PrepareConnection() error {
	// get token
	data, err := p.httpPost("/api/v1/bullet-public", nil)
	if err != nil {
		p.logger.Error().Err(err).Msg("fail getting token")
		return err
	}

	var response struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		p.logger.Error().Err(err).Msg("fail unmarshal token response")
		return err
	}

	p.wsEndpoint = fmt.Sprintf(
		"%s?token=%s&connectId=%d",
		p.config.WsEndpoints[0],
		response.Data.Token,
		time.Now().UnixMilli(),
	)

	return nil
}

func (p *KucoinProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var symbols []string
	for _, pair := range p.sortedPairs() {
		symbols = append(symbols, p.ToProviderSymbol(pair.Base, pair.Quote))
	}

	msgs := []map[string]any{{
		"id":       1,
		"type":     "subscribe",
		"topic":    "/market/snapshot:" + strings.Join(symbols, ","),
		"response": true,
	}}

	return msgs, nil
}

func (p *KucoinProvider) HandleWsMessage(msg []byte) error {
	var (
		marketMsg  KucoinWsMarketMsg
		pongMsg    KucoinWsPongMsg
		genericMsg KucoinWsGenericMsg
	)

	err := json.Unmarshal(msg, &marketMsg)
	if err == nil && marketMsg.Data.Data.Symbol != "" {
		data := marketMsg.Data.Data
		pair, found := p.pairs[data.Symbol]
		if !found {
			return nil
		}

		p.setTicker(
			time.Now(),
			pair,
			big.NewFloat(data.Price),
			big.NewFloat(data.Volume),
			nil,
		)

		return nil
	}

	err = json.Unmarshal(msg, &pongMsg)
	if err == nil && pongMsg.Type == "pong" {
		p.lastKeepalive = time.Now()
		return nil
	}

	err = json.Unmarshal(msg, &genericMsg)
	if err == nil && genericMsg.Type != "" {
		switch genericMsg.Type {
		case "welcome", "ack":
			return nil
		}
	}

	return ErrUnknownWsMsg
}

func (p *KucoinProvider) Ping() error {
	msg := map[string]any{
		"id":   fmt.Sprintf("%d", time.Now().UnixMilli()),
		"type": "ping",
	}

	return wsjson.Write(p.ctx, p.ws, msg)
}
