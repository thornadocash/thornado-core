package providers

import (
	"encoding/json"
	"strings"
	"time"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func (s *ProviderTestSuite) TestKucoinProvider(c *C) {
	config, found := s.config[common.ProviderKucoin]
	c.Assert(found, Equals, true)

	config.Pairs = []string{
		"BTC/USDT",
		"ETH/USDT",
		"RUJI/RUNE",
	}

	provider, err := NewKucoinProvider(s.ctx, s.logger, config, s.metrics)
	c.Assert(err, IsNil)
	c.Assert(provider, NotNil)
	c.Assert(provider.pairs, HasLen, 2)
	c.Assert(provider.tickers, HasLen, 0)

	symbol := provider.toMappedProviderSymbol(newPair("BTC/USDT"))
	c.Assert(err, IsNil)
	c.Assert(symbol, Equals, "BTC-USDT")

	// test polling

	provider.Poll()

	tickers, err := provider.GetTickers()
	c.Assert(err, IsNil)
	c.Assert(tickers, HasLen, 2)

	ticker, found := tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109013.90")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2652.63")

	// test subscription msg

	str := `[{"id":1,"response":true,"topic":"/market/snapshot:BTC-USDT,ETH-USDT","type":"subscribe"}]`

	msgs, err := provider.GetSubscriptionMsgs()
	c.Assert(err, IsNil)

	data, err := json.Marshal(msgs)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, str)

	// test connection url override

	/* trunk-ignore(golangci-lint/gosec) */
	token := "2neAiuYvAU61ZDXANAGAsiL4-iAExhsBXZxftpOeh_55i3Ysy2q2LEsEWU64mdzUOPusi34M_wGoSf7iNyEWJ6azopV1Fkhyh-QgaRrYOuDQZPYx4Ys3Z9iYB9J6i9GjsxUuhPw3Blq6rhZlGykT3Vp1phUafnulOOpts-MEmEFfDgE176QQ8boW21GeTS36JBvJHl5Vs9Y=.IxqhqVbk_kPXPr_RNv7Nhg=="
	err = provider.PrepareConnection()
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(provider.wsEndpoint, "?token="+token), Equals, true)

	// test websocket

	provider.Start()

	time.Sleep(time.Millisecond * 100)

	tickers, err = provider.GetTickers()
	c.Assert(err, IsNil)

	ticker, found = tickers["BTC/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "109132.10")

	ticker, found = tickers["ETH/USDT"]
	c.Assert(found, Equals, true)
	c.Assert(ticker.Price.Text('f', 2), Equals, "2657.23")

	err = provider.Ping()
	c.Assert(err, IsNil)
}
