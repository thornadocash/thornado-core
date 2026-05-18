package oracle

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
	. "gopkg.in/check.v1"
)

type ConverterTestSuite struct{}

var _ = Suite(&ConverterTestSuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *ConverterTestSuite) SetUpSuite(c *C) {}

func precision(s string) int {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return 0
	}
	return len(parts[1])
}

func newTicker(symbol string, price, volume float64) types.Ticker {
	if symbol == "" {
		symbol = "FOO/BAR"
	}
	parts := strings.Split(symbol, "/")
	return types.Ticker{
		Time:   time.Now(),
		Pair:   types.CurrencyPair{Base: parts[0], Quote: parts[1]},
		Price:  big.NewFloat(price),
		Volume: big.NewFloat(volume),
	}
}

func (s *ConverterTestSuite) TestVwap(c *C) {
	testCases := []struct {
		Tickers []types.Ticker
		Vwap    string
		Fail    bool
	}{
		{
			Tickers: []types.Ticker{
				newTicker("", 1, 3),
				newTicker("", 2, 2),
				newTicker("", 3, 3),
			},
			// 3 + 4 + 9 = 16 / 6
			Vwap: "2.0",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 1.234, 1),
			},
			Vwap: "1.234",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 1.234, 1),
				newTicker("", 1.234, 2),
			},
			Vwap: "1.234",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 1, 0),
				newTicker("", 2, 2),
				newTicker("", 3, 3),
			},
			Vwap: "2.6",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 2, 0),
				newTicker("", 3, 0),
			},
			Vwap: "0",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 0, 1),
				newTicker("", 3, 0),
			},
			Vwap: "0",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 0, 1),
				newTicker("", 3, 1),
			},
			Vwap: "3.0",
		},
		{
			Tickers: []types.Ticker{
				newTicker("", 0, 1),
				newTicker("", 3, 0),
				newTicker("", 9, 1),
			},
			Vwap: "9.0",
		},
		{
			Tickers: []types.Ticker{},
			Fail:    true,
		},
	}

	for _, tc := range testCases {
		values := map[string]*big.Float{}
		weights := map[string]*big.Float{}

		for i, ticker := range tc.Tickers {
			values[fmt.Sprintf("%d", i)] = new(big.Float).Copy(ticker.Price)
			weights[fmt.Sprintf("%d", i)] = new(big.Float).Copy(ticker.Volume)
		}

		vwap, err := ComputeVwap(values, weights)

		if tc.Fail {
			c.Assert(err, NotNil)
			continue
		}

		c.Assert(err, IsNil)
		c.Assert(vwap.Text('f', precision(tc.Vwap)), Equals, tc.Vwap)
	}
}

func (s *ConverterTestSuite) TestCappedPercent(c *C) {
	testCases := []struct {
		Threshold float64
		Precision int
		Values    []float64
		Result    []string
	}{
		{
			Threshold: 0.4,
			Precision: 1,
			Values:    []float64{50, 40, 10},
			Result:    []string{"0.4", "0.4", "0.2"},
		},
		{
			Threshold: 0.25,
			Precision: 2,
			Values:    []float64{40, 35, 10, 10, 5},
			Result:    []string{"0.25", "0.25", "0.20", "0.20", "0.10"},
		},
		{
			Threshold: 0.125,
			Precision: 3,
			Values:    []float64{100, 3, 2, 1, 0},
			Result:    []string{"0.125", "0.125", "0.125", "0.125", "0.000"},
		},
		{
			Threshold: 0,
			Precision: 0,
			Values:    []float64{99, 1},
			Result:    []string{"0", "0"},
		},
		{
			Threshold: 1,
			Precision: 2,
			Values:    []float64{1, 99},
			Result:    []string{"0.01", "0.99"},
		},
	}

	for _, tc := range testCases {
		values := map[string]*big.Float{}
		results := map[string]string{}

		for i, value := range tc.Values {
			key := fmt.Sprintf("%d", i)
			values[key] = big.NewFloat(value)
			results[key] = tc.Result[i]
		}

		threshold := big.NewFloat(tc.Threshold)

		for key, value := range ComputeCappedPercentage(values, threshold) {
			c.Assert(value.Text('f', tc.Precision), Equals, results[key])
		}
	}
}

func (s *ConverterTestSuite) TestUsdConversion(c *C) {
	// Test if USD rate calculation favors rates with higher volume
	// Convert USDT/BTC rates first (vol: ~1.75b USD)
	// This results in a USDT price of ~1.001 USD
	// Then calculate ETH/USDT (vol: ~456m USD, at the beginning of the round)

	tickersBySymbol := map[string]map[string]types.Ticker{
		"BTC/USD": {
			"coinbase": newTicker("BTC/USD", 100_000, 1),
			"crypto":   newTicker("BTC/USD", 100_000, 1),
			"kraken":   newTicker("BTC/USD", 100_000, 1),
		},
		"USDT/USD": {
			"kraken":   newTicker("USDT/USD", 1.002, 100_000),
			"coinbase": newTicker("USDT/USD", 1.002, 100_000),
			"bitfinex": newTicker("USDT/USD", 1.002, 100_000),
		},
		"USDT/BTC": {
			"binance": newTicker("USDT/BTC", 0.00001001, 200_000),
			"bitget":  newTicker("USDT/BTC", 0.00001001, 200_000),
			"gate":    newTicker("USDT/BTC", 0.00001001, 200_000),
		},
		"ETH/USDT": {
			"binance": newTicker("ETH/USDT", 2600, 7),
			"bitget":  newTicker("ETH/USDT", 2600, 2),
			"okx":     newTicker("ETH/USDT", 2600, 1),
		},
	}

	converter := NewConverter(tickersBySymbol, metrics.NewMetrics(false))

	// try to overwrite existing price
	converter.AddRate("BTC", "coinbase", newTicker("BTC/USD", 101_000, 1))

	rates, err := converter.ConvertToUsd()
	c.Assert(err, IsNil)
	c.Assert(len(rates), Equals, 3)
	// didn't overwrite coinbase BTC/USD rate
	c.Assert(rates["BTC"].Text('f', 1), Equals, "100000.0")
	// 1/3 USDT/USD: 1.002 (volume: 300k USDT)
	// 2/3 USDT/BTC with BTC @ 100k USD: 1.001 (volume: 600k USDT)
	c.Assert(rates["USDT"].Text('f', 8), Equals, "1.00133333")
	// USDT/USD updated with USDT/BTC pairs before converting, ETH/USDT
	// 2600 * 1.00133333 ~ 2603.4666
	c.Assert(rates["ETH"].Text('f', 3), Equals, "2603.467")

	// no prices for SOL
	price, volume, err := converter.GetRate("SOL", false)
	c.Assert(err, NotNil)
	c.Assert(price, IsNil)
	c.Assert(volume, IsNil)
}
