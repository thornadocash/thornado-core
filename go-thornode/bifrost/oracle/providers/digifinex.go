package providers

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type DigifinexTicker struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"last"`
	// 	BaseVolume  float64 `json:"vol"`
	// 	QuoteVolume float64 `json:"base_vol"`
}

type DigifinexWsTickerMsg struct {
	Method string `json:"method"`
	Params []struct {
		Symbol      string `json:"symbol"`
		Price       string `json:"last"`
		BaseVolume  string `json:"base_volume_24h"`
		QuoteVolume string `json:"quote_volume_24h"`
	}
}

type DigifinexWsSubsciptionMsg struct {
	Id     int64 `json:"id"`
	Result struct {
		Status string `json:"status"`
	} `json:"result"`
}

type DigifinexWsPongMsg struct {
	Result string `json:"result"`
}

type DigifinexProvider struct {
	*websocketProvider
}

func NewDigifinexProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*DigifinexProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderDigifinex, config, metrics)
	if err != nil {
		return nil, err
	}

	wsProvider.keepaliveInterval = time.Second * 30
	wsProvider.keepaliveTimeout = time.Second * 40

	provider := DigifinexProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *DigifinexProvider) getTickers() ([]DigifinexTicker, error) {
	data, err := p.httpGet("/v3/ticker")
	if err != nil {
		return nil, err
	}

	var response struct {
		Ticker []DigifinexTicker `json:"ticker"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Ticker, nil
}

func (p *DigifinexProvider) GetAvailablePairs() ([]string, error) {
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

func (p *DigifinexProvider) ToProviderSymbol(base, quote string) string {
	return strings.ToLower(base + "_" + quote)
}

// Polling

func (p *DigifinexProvider) Poll() {
	// not using Poll() because the current returned ticker data
	// seems to have base- and quote volume interchanged
}

// Websocket

func (p *DigifinexProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var symbols []string

	for _, pair := range p.sortedPairs() {
		symbols = append(symbols, p.ToProviderSymbol(pair.Base, pair.Quote))
	}

	msgs := []map[string]any{{
		"method": "ticker.subscribe",
		"id":     1,
		"params": symbols,
	}}

	return msgs, nil
}

func (p *DigifinexProvider) HandleWsMessage(msg []byte) error {
	zlibReader, err := zlib.NewReader(bytes.NewReader(msg))
	if err != nil {
		p.logger.Err(err).Msg("fail to create reader")
		return err
	}
	defer zlibReader.Close()

	reader := io.LimitReader(zlibReader, 10240)

	var out bytes.Buffer
	_, err = io.Copy(&out, reader)
	if err != nil {
		p.logger.Err(err).Msg("fail to decompress message")
		return err
	}

	msg = out.Bytes()

	var (
		tickerMsg       DigifinexWsTickerMsg
		subscriptionMsg DigifinexWsSubsciptionMsg
		pongMsg         DigifinexWsPongMsg
	)

	err = json.Unmarshal(msg, &tickerMsg)
	if err == nil && tickerMsg.Method == "ticker.update" {
		for _, ticker := range tickerMsg.Params {
			// rest reports lower case, ws upper case
			pair, found := p.pairs[strings.ToLower(ticker.Symbol)]
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
	if err == nil && subscriptionMsg.Id == 1 {
		return nil
	}

	err = json.Unmarshal(msg, &pongMsg)
	if err == nil && pongMsg.Result == "pong" {
		p.lastKeepalive = time.Now()
		return nil
	}

	return ErrUnknownWsMsg
}

func (p *DigifinexProvider) Ping() error {
	msg := map[string]any{
		"method": "server.ping",
		"id":     1,
	}

	return wsjson.Write(p.ctx, p.ws, msg)
}
