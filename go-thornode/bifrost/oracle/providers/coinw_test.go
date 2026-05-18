package providers

import (
	"encoding/json"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestCoinwProvider(c *C) {
	config, found := s.config[common.ProviderCoinw]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USDT",
		"ETH/USDT",
		"RUJI/RUNE",
	}

	provider, err := NewCoinwProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USDT"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "BTC_USDT")

	// test polling

	provider.Poll()
	// polling currently disabled
	c.Assert(provider.tickers, HasLen, 0)

	// test subscription msg

	// replace symbols with ids
	err = provider.PrepareConnection()
	c.Assert(err, IsNil)

	str := `[{"event":"sub","params":{"biz":"exchange","pairCode":"78","type":"ticker"}},{"event":"sub","params":{"biz":"exchange","pairCode":"79","type":"ticker"}}]`

	msgs, err := provider.GetSubscriptionMsgs()
	c.Assert(err, IsNil)

	data, err := json.Marshal(msgs)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, str)

	// test ws data
	provider.Start()

	time.Sleep(time.Millisecond * 100)

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	c.Assert(tickers, HasLen, 2)

	ticker, found := tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "108976.11")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2617.62")

	err = provider.Ping()
	c.Assert(err, IsNil)
}
