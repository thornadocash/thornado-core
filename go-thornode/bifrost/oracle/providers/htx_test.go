package providers

import (
	"encoding/json"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestHtxProvider(c *C) {
	config, found := s.config[common.ProviderHtx]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USDT",
		"ETH/USDT",
		"RUJI/RUNE",
	}

	provider, err := NewHtxProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USDT"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "btcusdt")

	// test polling

	provider.Poll()

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	c.Assert(tickers, HasLen, 2)

	ticker, found := tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109409.99")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2665.14")

	// test subscription msg

	str := `[{"sub":["market.btcusdt.ticker","market.ethusdt.ticker"]}]`

	msgs, err := provider.GetSubscriptionMsgs()
	c.Assert(err, IsNil)

	data, err := json.Marshal(msgs)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, str)

	// test websocket

	provider.Start()

	time.Sleep(time.Millisecond * 100)

	tickers, err = provider.GetTickers()
	c.Assert(err, IsNil)

	ticker, found = tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109425.00")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2665.20")
}
