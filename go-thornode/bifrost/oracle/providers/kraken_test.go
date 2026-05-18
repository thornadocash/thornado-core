package providers

import (
	"encoding/json"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestKrakenProvider(c *C) {
	config, found := s.config[common.ProviderKraken]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USD",
		"ETH/USD",
		"RUJI/RUNE",
	}

	config.SymbolMapping = []string{
		"BTC/USD=XXBTZUSD",
		"ETH/USD=XETHZUSD",
	}

	provider, err := NewKrakenProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USD"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "XXBTZUSD")

	// test polling

	provider.Poll()

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	c.Assert(tickers, HasLen, 2)

	ticker, found := tickers["BTC/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109337.20")

	ticker, found = tickers["ETH/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2660.46")

	// test subscription msg

	str := `[{"method":"subscribe","params":{"channel":"ticker","symbol":["BTC/USD","ETH/USD"]}}]`

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

	ticker, found = tickers["BTC/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109699.90")

	ticker, found = tickers["ETH/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2663.37")
}
