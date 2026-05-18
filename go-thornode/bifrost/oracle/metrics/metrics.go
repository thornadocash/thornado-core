package metrics

import (
	"math/big"

	"github.com/prometheus/client_golang/prometheus"
	"gitlab.com/thorchain/thornode/v3/bifrost/oracle/types"
)

type Metrics struct {
	detailed        bool
	providerRate    *prometheus.GaugeVec
	providerVolume  *prometheus.GaugeVec
	providerUpdate  *prometheus.CounterVec
	providerPrice   *prometheus.GaugeVec
	providerBound   *prometheus.GaugeVec
	converterVolume *prometheus.GaugeVec
}

func NewMetrics(detailed bool) *Metrics {
	metrics := Metrics{
		detailed: detailed,
		providerRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "oracle",
				Subsystem: "provider",
				Name:      "rate",
				Help:      "rate for trading pair reported by provider",
			}, []string{"provider", "base", "quote"},
		),
		providerVolume: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "oracle",
				Subsystem: "provider",
				Name:      "volume",
				Help:      "volume for trading pair reported by provider",
			}, []string{"provider", "base", "quote"},
		),
		providerUpdate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "oracle",
				Subsystem: "provider",
				Name:      "updates_total",
				Help:      "number of price updates for trading pair",
			}, []string{"provider", "base", "quote"},
		),
		providerPrice: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "oracle",
				Subsystem: "provider",
				Name:      "price",
				Help:      "calculated asset price in USD",
			}, []string{"provider", "symbol"},
		),
		providerBound: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "oracle",
				Subsystem: "provider",
				Name:      "bounds",
				Help:      "upper and lower bounds allowed for price calculation",
			}, []string{"symbol", "type"},
		),
		converterVolume: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "oracle",
				Subsystem: "converter",
				Name:      "volume_usd",
				Help:      "volume for asset calculated by converter",
			}, []string{"provider", "symbol", "type"},
		),
	}

	return &metrics
}

func (m *Metrics) Register() {
	_ = prometheus.Register(m.providerRate)
	_ = prometheus.Register(m.providerVolume)
	_ = prometheus.Register(m.providerUpdate)
	_ = prometheus.Register(m.providerPrice)
	_ = prometheus.Register(m.providerBound)
	_ = prometheus.Register(m.converterVolume)
}

func (m *Metrics) UpdateProviderTicker(provider string, ticker types.Ticker) {
	if !m.detailed {
		return
	}

	rate, _ := ticker.Price.Float64()
	volume, _ := ticker.Volume.Float64()

	base, quote := ticker.Pair.Base, ticker.Pair.Quote

	m.providerRate.WithLabelValues(provider, base, quote).Set(rate)
	m.providerVolume.WithLabelValues(provider, base, quote).Set(volume)
	m.providerUpdate.WithLabelValues(provider, base, quote).Add(1)
}

func (m *Metrics) UpdateProviderPrice(provider, symbol string, price *big.Float) {
	// always track final price
	if m.detailed || provider == "final" {
		value, _ := price.Float64()
		m.providerPrice.WithLabelValues(provider, symbol).Set(value)
	}
}

func (m *Metrics) UpdateConverterCappedVolume(provider, symbol string, volume *big.Float) {
	if !m.detailed {
		return
	}
	value, _ := volume.Float64()
	m.converterVolume.WithLabelValues(provider, symbol, "capped").Set(value)
}

func (m *Metrics) UpdateConverterTotalVolume(provider, symbol string, volume *big.Float) {
	if !m.detailed {
		return
	}
	value, _ := volume.Float64()
	m.converterVolume.WithLabelValues(provider, symbol, "total").Set(value)
}

func (m *Metrics) UpdateProviderBounds(symbol string, upper, lower *big.Float) {
	if !m.detailed {
		return
	}
	u, _ := upper.Float64()
	l, _ := lower.Float64()
	m.providerBound.WithLabelValues(symbol, "upper").Set(u)
	m.providerBound.WithLabelValues(symbol, "lower").Set(l)
}
