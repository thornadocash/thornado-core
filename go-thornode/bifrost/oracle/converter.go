package oracle

import (
	"fmt"
	"math/big"
	"sort"

	"gitlab.com/thorchain/thornode/v3/common"

	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
)

const (
	maxConversionSteps  = 3
	maxVolumePercentage = 0.4
	minProviders        = 3
	deviationFactor     = 5
)

type Converter struct {
	tickers map[string]map[string]types.Ticker // by symbol, provider: tickers[BTC/USDC][binance]
	rates   map[string]map[string]types.Ticker // by symbol, provider: rates[BTC][binance]
	metrics *metrics.Metrics
}

func NewConverter(
	tickers map[string]map[string]types.Ticker,
	metrics *metrics.Metrics,
) *Converter {
	return &Converter{
		tickers: tickers,
		rates:   map[string]map[string]types.Ticker{},
		metrics: metrics,
	}
}

func (c *Converter) AddRate(symbol, provider string, rate types.Ticker) {
	_, found := c.rates[symbol]
	if !found {
		c.rates[symbol] = map[string]types.Ticker{}
	}

	_, found = c.rates[symbol][provider]
	if found {
		// don't overwrite
		return
	}

	c.rates[symbol][provider] = rate
}

// GetRate returns the USD rate of the provided symbol by filtering known
// prices with regard to maxDeviation and computing the volume weighted average
func (c *Converter) GetRate(symbol string, metrics bool) (*big.Float, *big.Float, error) {
	rates, found := c.rates[symbol]
	if metrics {
		for provider, rate := range rates {
			c.metrics.UpdateProviderPrice(provider, symbol, rate.Price)
		}
	}

	if !found || len(rates) < minProviders {
		return nil, nil, fmt.Errorf("not enough rates")
	}

	rates, lowerBound, upperBound, err := FilterDeviations(rates)
	if err != nil {
		return nil, nil, err
	}

	if metrics {
		c.metrics.UpdateProviderBounds(symbol, upperBound, lowerBound)
	}

	if len(rates) == 0 {
		return nil, nil, fmt.Errorf("no rates found")
	}

	volume := new(big.Float)

	prices := map[string]*big.Float{}
	totalVolumes := map[string]*big.Float{}
	cappedVolumes := map[string]*big.Float{}

	for provider, rate := range rates {
		prices[provider] = new(big.Float).Copy(rate.Price)
		totalVolumes[provider] = new(big.Float).Copy(rate.Volume)
		volume.Add(volume, rate.Volume)
	}

	// temporary hack for RUNE until on chain volume data is available:
	// set the volume reported by thorchain to twice the amount of the highest
	// reported volume
	_, found = totalVolumes[common.ProviderThorchain]
	if symbol == "RUNE" && found {
		maxVolume := new(big.Float)
		for _, providerVolume := range totalVolumes {
			if providerVolume.Cmp(maxVolume) > 0 {
				maxVolume = new(big.Float).Copy(providerVolume)
			}
		}
		newVolume := new(big.Float).Mul(maxVolume, big.NewFloat(2))
		totalVolumes[common.ProviderThorchain] = newVolume
		volume.Add(volume, newVolume)
	}

	// replace volume totals with capped percentages
	threshold := big.NewFloat(maxVolumePercentage)
	for provider, percent := range ComputeCappedPercentage(totalVolumes, threshold) {
		cappedVolumes[provider] = new(big.Float).Copy(percent)
	}

	if metrics {
		for provider := range c.rates[symbol] {
			total, ok := totalVolumes[provider]
			if !ok {
				total = new(big.Float)
			}

			capped, ok := cappedVolumes[provider]
			if !ok {
				capped = new(big.Float)
			}

			c.metrics.UpdateConverterTotalVolume(provider, symbol, total)
			c.metrics.UpdateConverterCappedVolume(provider, symbol, capped)
		}
	}

	rate, err := ComputeVwap(prices, cappedVolumes)
	if err != nil {
		return nil, nil, err
	}

	if metrics {
		c.metrics.UpdateProviderPrice("final", symbol, rate)
	}

	// estimate volume in USD
	volume.Mul(volume, rate)

	return rate, volume, nil
}

func (c *Converter) ConvertToUsd() (map[string]*big.Float, error) {
	pairsByQuote := map[string][]types.CurrencyPair{}
	symbols := map[string]struct{}{}
	for _, tickers := range c.tickers {
		for provider, ticker := range tickers {
			if ticker.Pair.Base == "USD" {
				// we are looking for the base price in USD,
				// this wouldn't make sense
				continue
			}

			quote := ticker.Pair.Quote

			if quote == "USD" {
				c.AddRate(ticker.Pair.Base, provider, ticker)
				continue
			}

			_, found := pairsByQuote[quote]
			if !found {
				pairsByQuote[quote] = []types.CurrencyPair{}
			}

			// dedup ticker symbols
			symbol := ticker.Pair.String()
			_, found = symbols[symbol]
			if found {
				continue
			}
			symbols[symbol] = struct{}{}

			pairsByQuote[quote] = append(pairsByQuote[quote], ticker.Pair)
		}
	}

	for round := 0; round < maxConversionSteps; round++ {
		var rates []types.Ticker
		for denom := range c.rates {
			rate, volume, err := c.GetRate(denom, false)
			if err != nil {
				continue
			}
			rates = append(rates, types.Ticker{
				Pair:   types.CurrencyPair{Base: denom, Quote: "USD"},
				Price:  rate,
				Volume: new(big.Float).Mul(volume, rate),
			})
		}

		// sort rates by volume, so we convert pairs with the highest volume for
		// the quote asset first

		sort.Slice(rates, func(i, j int) bool {
			return rates[i].Volume.Cmp(rates[j].Volume) > 0
		})

		for i := range rates {
			// rate is BTC/USD, so we are looking for x/BTC pairs
			quote := rates[i].Pair.Base
			pairs, found := pairsByQuote[quote]
			if !found {
				continue
			}

			// get fresh rate, in case it was updated  during this round
			rate, _, err := c.GetRate(quote, false)
			if err != nil {
				continue
			}

			for _, pair := range pairs {
				symbol := pair.String()
				base := pair.Base

				// get all tickers for the current symbol
				tickers := c.tickers[symbol]
				for provider, ticker := range tickers {
					c.AddRate(base, provider, types.Ticker{
						Pair:   types.CurrencyPair{Base: base, Quote: quote},
						Price:  new(big.Float).Mul(ticker.Price, rate),
						Volume: new(big.Float).Copy(ticker.Volume),
						Time:   ticker.Time,
					})
				}
			}

			delete(pairsByQuote, quote)
		}

		// Stop if all pairs could be resolved
		if len(pairsByQuote) == 0 {
			break
		}
	}

	// get all rates

	rates := map[string]*big.Float{}

	for denom := range c.rates {
		rate, _, err := c.GetRate(denom, true)
		if err != nil {
			continue
		}
		rates[denom] = new(big.Float).Copy(rate)
	}

	return rates, nil
}

// ----------------------------------------------------------------------------

func FilterDeviations(
	tickers map[string]types.Ticker,
) (map[string]types.Ticker, *big.Float, *big.Float, error) {
	values := []*big.Float{}
	for _, ticker := range tickers {
		values = append(values, ticker.Price)
	}

	deviation, median, err := common.MedianAbsoluteDeviation(values)
	if err != nil {
		return nil, nil, nil, err
	}

	margin := new(big.Float).Mul(deviation, big.NewFloat(deviationFactor))

	lowerBound := new(big.Float).Sub(median, margin)
	upperBound := new(big.Float).Add(median, margin)

	filtered := map[string]types.Ticker{}
	for provider, ticker := range tickers {
		price := ticker.Price
		if price.Cmp(lowerBound) == -1 || price.Cmp(upperBound) == 1 {
			continue
		}
		filtered[provider] = ticker
	}

	return filtered, lowerBound, upperBound, nil
}

func ComputeVwap(prices, volumes map[string]*big.Float) (*big.Float, error) {
	if len(prices) == 0 || len(volumes) == 0 {
		return nil, fmt.Errorf("not enough values for calculation")
	}

	if len(prices) != len(volumes) {
		return nil, fmt.Errorf("amount of values and weights don't match")
	}

	weightedTotal := new(big.Float)
	totalVolume := new(big.Float)

	for k, rate := range prices {

		if rate.Cmp(new(big.Float)) <= 0 {
			continue
		}

		weightedPrice := new(big.Float).Mul(rate, volumes[k])
		weightedTotal.Add(weightedTotal, weightedPrice)
		totalVolume.Add(totalVolume, volumes[k])
	}

	// zero
	if totalVolume.Cmp(new(big.Float)) == 0 {
		totalVolume = big.NewFloat(1)
	}

	return new(big.Float).Quo(weightedTotal, totalVolume), nil
}

func ComputePercentage(
	values map[string]*big.Float,
) map[string]*big.Float {
	total := new(big.Float)
	for _, value := range values {
		total.Add(total, value)
	}

	if total.Cmp(new(big.Float)) == 0 {
		return values
	}

	percentage := map[string]*big.Float{}
	for provider, value := range values {
		percentage[provider] = new(big.Float).Quo(value, total)
	}

	return percentage
}

func ComputeCappedPercentage(
	values map[string]*big.Float,
	cap *big.Float,
) map[string]*big.Float {
	return RedistributePercentage(ComputePercentage(values), new(big.Float), cap)
}

func RedistributePercentage(
	values map[string]*big.Float,
	amount, cap *big.Float,
) map[string]*big.Float {
	remain := new(big.Float)

	percentages := ComputePercentage(values)
	capped := map[string]*big.Float{}

	for k, v := range values {
		share := new(big.Float).Mul(percentages[k], amount)
		value := new(big.Float).Add(v, share)
		if value.Cmp(cap) >= 0 {
			remain.Add(remain, new(big.Float).Sub(value, cap))
			capped[k] = new(big.Float).Copy(cap)
			delete(values, k)
		} else {
			values[k].Add(v, share)
		}
	}

	if remain.Cmp(new(big.Float)) > 0 && len(values) > 0 {
		for k, v := range RedistributePercentage(values, remain, cap) {
			capped[k] = new(big.Float).Copy(v)
		}
	}

	for k, v := range values {
		capped[k] = new(big.Float).Copy(v)
	}

	return capped
}
