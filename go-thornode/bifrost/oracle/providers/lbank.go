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
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type LbankTicker struct {
	Symbol string `json:"symbol"`
	Ticker struct {
		Price  string `json:"latest"`
		Volume string `json:"vol"`
	} `json:"ticker"`
}

type LbankWsTickerMsg struct {
	Pair string `json:"pair"`
	Tick struct {
		Price  float64 `json:"latest"`
		Volume float64 `json:"vol"`
	} `json:"tick"`
}

type LbankWsPingMsg struct {
	Action string `json:"action"`
	Ping   string `json:"ping"`
}

type LbankProvider struct {
	*websocketProvider
}

func NewLbankProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*LbankProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderLbank, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := LbankProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *LbankProvider) GetAvailablePairs() ([]string, error) {
	data, err := p.httpGet("/v2/currencyPairs.do")
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []string `json:"data"`
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (p *LbankProvider) ToProviderSymbol(base, quote string) string {
	return strings.ToLower(base + "_" + quote)
}

// Polling

func (p *LbankProvider) Poll() {
	reqsPerSecond := 15 // staying a bit below 20 for safety
	i := 0
	for symbol, pair := range p.pairs {
		go func(p *LbankProvider, symbol string, pair types.CurrencyPair) {
			path := fmt.Sprintf("/v2/ticker/24hr.do?symbol=%s", symbol)
			data, err := p.httpGet(path)
			if err != nil {
				return
			}

			var response struct {
				Data [1]LbankTicker `json:"data"`
			}

			err = json.Unmarshal(data, &response)
			if err != nil {
				return
			}

			ticker := response.Data[0].Ticker

			p.setTicker(
				time.Now(),
				pair,
				strToFloat(ticker.Price),
				strToFloat(ticker.Volume),
				nil,
			)
		}(p, symbol, pair)
		// LBank has a rate limit of 20req/s, sleeping 1.2s before running
		// the next batch of requests
		i++
		if i == reqsPerSecond {
			i = 0
			time.Sleep(time.Millisecond * 1200)
		}
	}
}

// Websocket

func (p *LbankProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	msgs := []map[string]any{}

	for _, pair := range p.sortedPairs() {
		msgs = append(msgs, map[string]any{
			"action":    "subscribe",
			"subscribe": "tick",
			"pair":      p.toMappedProviderSymbol(pair),
		})
	}

	return msgs, nil
}

func (p *LbankProvider) HandleWsMessage(msg []byte) error {
	var (
		tickerMsg LbankWsTickerMsg
		pingMsg   LbankWsPingMsg
	)

	err := json.Unmarshal(msg, &tickerMsg)
	if err == nil && tickerMsg.Pair != "" {
		pair, found := p.pairs[tickerMsg.Pair]
		if !found {
			return nil
		}

		p.setTicker(
			time.Now(),
			pair,
			big.NewFloat(tickerMsg.Tick.Price),
			big.NewFloat(tickerMsg.Tick.Volume),
			nil,
		)

		return nil
	}

	err = json.Unmarshal(msg, &pingMsg)
	if err == nil && pingMsg.Action == "ping" {
		msg := map[string]any{
			"action": "pong",
			"pong":   pingMsg.Ping,
		}

		err = wsjson.Write(p.ctx, p.ws, msg)
		if err != nil {
			p.logger.Err(err).Msg("fail to subscribe")
			return err
		}

		return nil
	}

	return ErrUnknownWsMsg
}
