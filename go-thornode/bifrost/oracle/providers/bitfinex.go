package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

type BitfinexTicker struct {
	Symbol string
	Price  float64
	Volume float64
}

type BitfinexWsResponseMsg struct {
	Event     string `json:"event"`
	Channel   string `json:"channel"`
	ChannelId int64  `json:"chanId"`
	Symbol    string `json:"symbol"`
	Pair      string `json:"pair"`
}

type BitfinexProvider struct {
	*websocketProvider
}

func NewBitfinexProvider(
	ctx context.Context,
	logger zerolog.Logger,
	config config.BifrostOracleProviderConfiguration,
	metrics *metrics.Metrics,
) (*BitfinexProvider, error) {
	wsProvider, err := newWebsocketProvider(ctx, logger, common.ProviderBitfinex, config, metrics)
	if err != nil {
		return nil, err
	}

	provider := BitfinexProvider{wsProvider}
	err = provider.init(&provider)
	if err != nil {
		return nil, err
	}

	return &provider, nil
}

func (p *BitfinexProvider) getTickers() ([][]any, error) {
	data, err := p.httpGet("/v2/tickers?symbols=ALL")
	if err != nil {
		return nil, err
	}

	var values [][]any
	err = json.Unmarshal(data, &values)
	if err != nil {
		return nil, err
	}

	return values, nil
}

func (p *BitfinexProvider) GetAvailablePairs() ([]string, error) {
	tickers, err := p.getTickers()
	if err != nil {
		return nil, err
	}

	symbols := []string{}
	for _, ticker := range tickers {
		symbol, ok := ticker[0].(string)
		if !ok {
			continue
		}
		if !strings.HasPrefix(symbol, "t") {
			continue
		}

		symbol = strings.Replace(symbol, ":", "", 1)
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

func (p *BitfinexProvider) ToProviderSymbol(base, quote string) string {
	return "t" + base + quote
}

func (p *BitfinexProvider) Poll() {
	tickers, err := p.getTickers()
	if err != nil {
		p.logger.Err(err).Msg("fail to get tickers")
		return
	}

	for _, ticker := range tickers {
		if len(ticker) != 11 {
			// trading pairs have exactly 11 items
			continue
		}

		symbol, ok := ticker[0].(string)
		if !ok {
			continue
		}

		pair, found := p.pairs[symbol]
		if !found {
			continue
		}

		price, ok := ticker[7].(float64)
		if !ok {
			continue
		}

		volume, ok := ticker[8].(float64)
		if !ok {
			continue
		}

		p.setTicker(
			time.Now(),
			pair,
			big.NewFloat(price),
			big.NewFloat(volume),
			nil,
		)
	}
}

func (p *BitfinexProvider) GetSubscriptionMsgs() ([]map[string]any, error) {
	msgs := []map[string]any{}

	for _, pair := range p.sortedPairs() {
		symbol := p.toMappedProviderSymbol(pair)

		msgs = append(msgs, map[string]any{
			"event":   "subscribe",
			"channel": "ticker",
			"symbol":  symbol,
		})
	}

	return msgs, nil
}

func (p *BitfinexProvider) HandleWsMessage(msg []byte) error {
	var responseMsg BitfinexWsResponseMsg
	var tickerMsg []any

	err := json.Unmarshal(msg, &responseMsg)
	if err == nil && responseMsg.Event != "" {
		if responseMsg.Event != "subscribed" {
			return nil
		}

		// further update messages will only provide the channel id, so replace
		// the symbol with the channel id

		symbol := responseMsg.Symbol
		pair, found := p.pairs[symbol]
		if !found {
			return nil
		}
		delete(p.pairs, symbol)
		p.pairs[fmt.Sprintf("%d", responseMsg.ChannelId)] = pair

		return nil
	}

	err = json.Unmarshal(msg, &tickerMsg)
	if err == nil {
		channelId, ok := tickerMsg[0].(float64)
		if !ok {
			p.logger.Warn().Msg("fail to parse ticker")
			return nil
		}

		values, ok := tickerMsg[1].([]any)
		if !ok {
			// check for heartbeat
			_, ok = tickerMsg[1].(string)
			if ok {
				return nil
			}
			p.logger.Warn().Msg("fail to parse ticker")
			return nil
		}

		if len(values) != 10 {
			p.logger.Warn().Msg("fail to parse ticker")
			return nil
		}

		pair, found := p.pairs[fmt.Sprintf("%d", int64(channelId))]
		if !found {
			p.logger.Warn().Msg("pair not found")
			return nil
		}

		price, ok := values[6].(float64)
		if !ok {
			p.logger.Warn().Msg("fail to parse ticker")
			return nil
		}

		volume, ok := values[7].(float64)
		if !ok {
			p.logger.Warn().Msg("fail to parse ticker")
			return nil
		}

		p.setTicker(
			time.Now(),
			pair,
			big.NewFloat(price),
			big.NewFloat(volume),
			nil,
		)

		return nil
	}

	return ErrUnknownWsMsg
}
