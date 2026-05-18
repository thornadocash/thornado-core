package providers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type CryptoTicker struct {
	Time   int64  `json:"t"`
	Symbol string `json:"i"`
	Price  string `json:"a"`
	Volume string `json:"v"`
}

type CryptoWsTickersResponse struct {
	Result struct {
		Data []struct {
			Symbol string `json:"i"`
			Price  string `json:"a"`
			Volume string `json:"v"`
		}
	} `json:"result"`
}

type CryptoWsHeartbeat struct {
	Id     uint64 `json:"id"`
	Method string `json:"method"`
	Code   int    `json:"code"`
}

type CryptoProvider struct {
	*websocketProvider
}

func NewCryptoProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*CryptoProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderCrypto, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := CryptoProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *CryptoProvider) getTickers() ([]CryptoTicker, error) {
	data, err := p.httpGet("/exchange/v1/public/get-tickers")
	if err != nil {
		return nil, err
	}

	var response struct {
		Result struct {
			Data []CryptoTicker `json:"data"`
		} `json:"result"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Result.Data, nil
}

func (p *CryptoProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return nil, err
	}

	symbols := []string{}
	for _, ticker := range tickers {
		if strings.HasSuffix(ticker.Symbol, "-PERP") {
			continue
		}

		symbols = append(symbols, ticker.Symbol)
	}

	return symbols, nil
}

func (p *CryptoProvider) Poll() {
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
			time.UnixMilli(ticker.Time),
			pair,
			strToFloat(ticker.Price),
			strToFloat(ticker.Volume),
			nil,
		)
	}
}

func (p *CryptoProvider) ToProviderSymbol(base, quote string) string {
	return base + "_" + quote
}

// Websocket

func (p *CryptoProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	var channels []string

	for _, pair := range p.sortedPairs() {
		channels = append(channels, "ticker."+pair.Join("_"))
	}

	msgs := []map[string]any{{
		"method": "subscribe",
		"params": map[string]any{
			"channels": channels,
		},
		"id":    1,
		"nonce": 1,
	}}

	return msgs, nil
}

func (p *CryptoProvider) HandleWsMessage(msg []byte) error {
	var response CryptoWsTickersResponse

	err := json.Unmarshal(msg, &response)
	if err == nil && response.Result.Data != nil {
		for _, data := range response.Result.Data {

			pair, found := p.pairs[data.Symbol]
			if !found {
				return nil
			}

			timestamp := time.Now()

			p.setTicker(
				timestamp,
				pair,
				strToFloat(data.Price),
				strToFloat(data.Volume),
				nil,
			)
		}

		return nil
	}

	var heartbeat CryptoWsHeartbeat

	err = json.Unmarshal(msg, &heartbeat)
	if err == nil {
		r := map[string]any{
			"id":     heartbeat.Id,
			"method": "public/respond-heartbeat",
		}
		err = wsjson.Write(p.ctx, p.ws, r)
		if err != nil {
			p.logger.Err(err).Msg("fail to send heartbeat response")
			return err
		}
		return nil
	}

	p.logger.Err(err).
		Str("msg", string(msg)).
		Msg("fail to parse ws message")

	return err
}
