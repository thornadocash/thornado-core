package providers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type HtxTicker struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"close"`
	Volume float64 `json:"amount"`
}

type HtxWsTickerMsg struct {
	Channel string `json:"ch"`
	Tick    struct {
		Price  float64 `json:"lastPrice"`
		Volume float64 `json:"amount"`
	} `json:"tick"`
}

type HtxWsPingMsg struct {
	Ping uint64 `json:"ping"`
}

type HtxProvider struct {
	*websocketProvider
}

func NewHtxProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*HtxProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderHtx, config, metrics)
	if err != nil {
		return nil, err
	}

	// restart if we haven't received any ping in the last 30s
	wsProvider.keepaliveTimeout = time.Second * 30

	provider := HtxProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *HtxProvider) getTickers() ([]HtxTicker, error) {
	data, err := p.httpGet("/market/tickers")
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []HtxTicker `json:"data"`
	}
	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (p *HtxProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, ticker := range tickers {
		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *HtxProvider) ToProviderSymbol(base, quote string) string {
	return strings.ToLower(base + quote)
}

func (p *HtxProvider) Poll() {
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
			big.NewFloat(ticker.Price),
			big.NewFloat(ticker.Volume),
			nil,
		)
	}
}

func (p *HtxProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var subscriptions []string

	for _, pair := range p.sortedPairs() {
		symbol := p.toMappedProviderSymbol(pair)
		subscriptions = append(
			subscriptions,
			fmt.Sprintf("market.%s.ticker", symbol),
		)
	}

	msgs := []map[string]any{{
		"sub": subscriptions,
	}}

	return msgs, nil
}

func (p *HtxProvider) HandleWsMessage(msg []byte) error {
	var limit int64 = 10240 // 10 KB

	// ws data from htx is compressed with GZIP
	gzipReader, err := gzip.NewReader(bytes.NewReader(msg))
	if err != nil {
		p.logger.Err(err).Msg("fail to create gzip reader")
		return err
	}
	defer gzipReader.Close()

	reader := io.LimitReader(gzipReader, limit)

	var data bytes.Buffer
	_, err = io.Copy(&data, reader)
	if err != nil {
		p.logger.Err(err).Msg("fail to read data")
		return err
	}

	msg = data.Bytes()

	var tickerMsg HtxWsTickerMsg
	var pingMsg HtxWsPingMsg

	err = json.Unmarshal(msg, &tickerMsg)
	// analyze-ignore(float-comparison)
	if err == nil && tickerMsg.Tick.Price > 0 {
		parts := strings.Split(tickerMsg.Channel, ".")
		if len(parts) != 3 {
			p.logger.Warn().Str("channel", tickerMsg.Channel).Msg("fail to parse channel")
			return nil
		}

		symbol := parts[1]
		ticker := tickerMsg.Tick

		pair, found := p.pairs[symbol]
		if !found {
			return nil
		}

		p.setTicker(
			time.Now(),
			pair,
			big.NewFloat(ticker.Price),
			big.NewFloat(ticker.Volume),
			nil,
		)

		return nil
	}

	err = json.Unmarshal(msg, &pingMsg)
	if err == nil && pingMsg.Ping > 0 {
		err = wsjson.Write(p.ctx, p.ws, map[string]any{
			"pong": pingMsg.Ping,
		})
		if err != nil {
			p.logger.Err(err).Msg("fail to write pong")
			return err
		}
		p.lastKeepalive = time.Now()
	}

	return nil
}
