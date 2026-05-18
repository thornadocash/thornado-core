package providers

import (
	"encoding/json"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestCryptoProvider(c *C) {
	config, found := s.config[common.ProviderCrypto]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USD",
		"ETH/USD",
		"RUJI/RUNE",
	}

	provider, err := NewCryptoProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USD"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "BTC_USD")

	// test polling

	provider.Poll()

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	c.Assert(tickers, HasLen, 2)

	ticker, found := tickers["BTC/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "108779.99")

	ticker, found = tickers["ETH/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2622.85")

	// test subscription msg

	str := `[{"id":1,"method":"subscribe","nonce":1,"params":{"channels":["ticker.BTC_USD","ticker.ETH_USD"]}}]`

	msgs, err := provider.GetSubscriptionMsgs()
	c.Assert(err, IsNil)

	data, err := json.Marshal(msgs)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, str)

	c.Assert(msgs, HasLen, 1)
	c.Assert(msgs[0], HasLen, 4)

	// test websocket

	provider.Start()

	time.Sleep(time.Millisecond * 100)

	tickers, err = provider.GetTickers()
	c.Assert(err, IsNil)

	ticker, found = tickers["BTC/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "108620.00")

	ticker, found = tickers["ETH/USD"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2611.21")
}
