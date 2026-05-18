package providers

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type OkxTicker struct {
	Symbol    string `json:"instId"`
	Price     string `json:"last"`
	Volume    string `json:"vol24h"`
	Timestamp string `json:"ts"`
	Type      string `json:"instType"`
}

type OkxWsTickerMsg struct {
	Data []OkxTicker `json:"data"`
}

type OkxWsEventMsg struct {
	Event string `json:"event"`
}

type OkxProvider struct {
	*websocketProvider
}

func NewOkxProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*OkxProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderOkx, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := OkxProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *OkxProvider) getTickers() ([]OkxTicker, error) {
	data, err := p.httpGet("/api/v5/market/tickers?instType=SPOT")
	if err != nil {
		p.logger.Err(err).Msg("error fetching data")
		return nil, err
	}

	var response struct {
		Data []OkxTicker `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (p *OkxProvider) GetAvailablePairs() ([]string, error) {
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

func (p *OkxProvider) ToProviderSymbol(base, quote string) string {
	return base + "-" + quote
}

// Polling

func (p *OkxProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("error fetching tickers")
		return
	}

	for _, ticker := range tickers {
		pair, found := p.pairs[ticker.Symbol]
		if !found {
			continue
		}

		msecs, err := strconv.ParseInt(ticker.Timestamp, 10, 64)
		if err != nil {
			p.logger.Err(err).Msg("error parsing timestamp")
			continue
		}

		timestamp := time.UnixMilli(msecs)

		p.setTicker(
			timestamp,
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.Volume),
			nil,
		)
	}
}

// Websocket

func (p *OkxProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var args []map[string]string

	for _, pair := range p.sortedPairs() {
		args = append(args, map[string]string{
			"channel": "tickers",
			"instId":  pair.Join("-"),
		})
	}

	msgs := []map[string]any{{
		"op":   "subscribe",
		"args": args,
		"id":   1,
	}}

	return msgs, nil
}

func (p *OkxProvider) HandleWsMessage(msg []byte) error {
	var tickerMsg OkxWsTickerMsg
	var eventMsg OkxWsEventMsg

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && len(tickerMsg.Data) > 0 {
		for _, ticker := range tickerMsg.Data {
			pair, found := p.pairs[ticker.Symbol]
			if !found {
				return nil
			}

			var msecs int64
			msecs, err = strconv.ParseInt(ticker.Timestamp, 10, 64)
			if err != nil {
				p.logger.Err(err).Msg("error parsing timestamp")
				continue
			}

			timestamp := time.UnixMilli(msecs)

			p.setTicker(
				timestamp,
				pair,
				strToFloat(ticker.Price),
				strToFloat(ticker.Volume),
				nil,
			)
		}

		return nil
	}

	err = json.Unmarshal(msg, &eventMsg)
	if err == nil && eventMsg.Event == "subscribe" {
		// subscription response
		return nil
	}

	return ErrUnknownWsMsg
}
