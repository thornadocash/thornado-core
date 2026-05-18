package providers

import (
	"encoding/json"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestBitmartProvider(c *C) {
	config, found := s.config[common.ProviderBitmart]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USDT",
		"ETH/USDT",
		"RUJI/RUNE",
	}

	provider, err := NewBitmartProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USDT"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "BTC_USDT")

	// test polling

	provider.Poll()

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	// supports reversed pairs
	c.Assert(tickers, HasLen, 4)

	ticker, found := tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "108726.01")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2606.51")

	ticker, found = tickers["USDT/BTC"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 8), Equals, "0.00000920")

	// test subscription msg

	str := `[{"args":["spot/ticker:BTC_USDT","spot/ticker:ETH_USDT"],"op":"subscribe"}]`

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
	c.Assert(ticker.Price.Text('f', 2), Equals, "108717.98")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2604.65")

	err = provider.Ping()
	c.Assert(err, IsNil)
}
