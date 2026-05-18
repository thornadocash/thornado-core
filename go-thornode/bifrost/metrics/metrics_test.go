package metrics

import (
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
)

// ---------------------------------------------------------------------------
// chain_metrics.go — metric name helpers
// ---------------------------------------------------------------------------

func TestSignAndBroadcastDuration(t *testing.T) {
	got := SignAndBroadcastDuration(common.BTCChain)
	exp := MetricName(common.BTCChain + "_sign_and_broadcast_duration")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestTxSigned(t *testing.T) {
	got := TxSigned(common.ETHChain)
	exp := MetricName(common.ETHChain + "_tx_signed")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestTxSignedBroadcast(t *testing.T) {
	got := TxSignedBroadcast(common.ETHChain)
	exp := MetricName(common.ETHChain + "_tx_signed_broadcast")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestBlockScanError(t *testing.T) {
	got := BlockScanError(common.BTCChain)
	exp := MetricName(common.BTCChain + "_block_scan_error")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestBlockWithoutTx(t *testing.T) {
	got := BlockWithoutTx(common.BTCChain)
	exp := MetricName(common.BTCChain + "_block_without_tx")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestBlockWithTxIn(t *testing.T) {
	got := BlockWithTxIn(common.BTCChain)
	exp := MetricName(common.BTCChain + "_block_with_tx_in")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestBlockNoTxIn(t *testing.T) {
	got := BlockNoTxIn(common.BTCChain)
	exp := MetricName(common.BTCChain + "_block_no_tx_in")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestBlockNoTxOut(t *testing.T) {
	got := BlockNoTxOut(common.BTCChain)
	exp := MetricName(common.BTCChain + "_block_no_tx_out")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestSearchTxDuration(t *testing.T) {
	got := SearchTxDuration(common.ETHChain)
	exp := MetricName(common.ETHChain + "_search_tx_duration")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestGasPrice(t *testing.T) {
	got := GasPrice(common.ETHChain)
	exp := MetricName(common.ETHChain + "_gas_price")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestGasPriceChange(t *testing.T) {
	got := GasPriceChange(common.ETHChain)
	exp := MetricName(common.ETHChain + "_gas_price_change")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

func TestGasPriceSuggested(t *testing.T) {
	got := GasPriceSuggested(common.ETHChain)
	exp := MetricName(common.ETHChain + "_gas_price_suggested")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

// ---------------------------------------------------------------------------
// chain_metrics.go — AddChainMetrics
// ---------------------------------------------------------------------------

func TestAddChainMetrics(t *testing.T) {
	c := make(map[MetricName]prometheus.Counter)
	cv := make(map[MetricName]*prometheus.CounterVec)
	g := make(map[MetricName]prometheus.Gauge)
	h := make(map[MetricName]prometheus.Histogram)

	chain := common.BTCChain
	AddChainMetrics(chain, c, cv, g, h)

	// counters: 7 entries
	expectedCounters := []MetricName{
		BlockWithoutTx(chain),
		BlockWithTxIn(chain),
		BlockNoTxIn(chain),
		BlockNoTxOut(chain),
		GasPriceChange(chain),
		TxSignedBroadcast(chain),
		TxSigned(chain),
	}
	for _, name := range expectedCounters {
		if _, ok := c[name]; !ok {
			t.Errorf("missing counter %s", name)
		}
	}

	// counterVecs: 1 entry
	if _, ok := cv[BlockScanError(chain)]; !ok {
		t.Error("missing counter vec BlockScanError")
	}

	// histograms: 2 entries
	expectedHistograms := []MetricName{
		SearchTxDuration(chain),
		SignAndBroadcastDuration(chain),
	}
	for _, name := range expectedHistograms {
		if _, ok := h[name]; !ok {
			t.Errorf("missing histogram %s", name)
		}
	}

	// gauges: 2 entries
	expectedGauges := []MetricName{
		GasPrice(chain),
		GasPriceSuggested(chain),
	}
	for _, name := range expectedGauges {
		if _, ok := g[name]; !ok {
			t.Errorf("missing gauge %s", name)
		}
	}
}

// ---------------------------------------------------------------------------
// metrics.go — NewMetrics (must run first since prometheus registration is
// one-shot per process)
// ---------------------------------------------------------------------------

func TestNewMetrics(t *testing.T) {
	cfg := config.BifrostMetricsConfiguration{
		Enabled:      false,
		ListenPort:   9999,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Chains:       []common.Chain{common.ETHChain},
	}
	m, err := NewMetrics(cfg)
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}

	// Verify chain metrics were added
	if c := m.GetCounter(TxSigned(common.ETHChain)); c == nil {
		t.Error("expected chain counter TxSigned(ETH) to be registered")
	}
	if g := m.GetGauge(GasPrice(common.ETHChain)); g == nil {
		t.Error("expected chain gauge GasPrice(ETH) to be registered")
	}
	if h := m.GetHistograms(SearchTxDuration(common.ETHChain)); h == nil {
		t.Error("expected chain histogram SearchTxDuration(ETH) to be registered")
	}
	if cv := m.GetCounterVec(BlockScanError(common.ETHChain)); cv == nil {
		t.Error("expected chain counter vec BlockScanError(ETH) to be registered")
	}
}

func TestNewMetrics_PprofEnabled(t *testing.T) {
	// We can't call NewMetrics twice (duplicate prometheus registration), so
	// just verify the struct was created properly. The pprof branch simply
	// adds handlers — we test that Start/Stop works with pprof-enabled config.
	cfg := config.BifrostMetricsConfiguration{
		Enabled:      true,
		PprofEnabled: true,
		ListenPort:   0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	// Build a Metrics manually with pprof-configured server to cover Start path
	server := http.NewServeMux()
	server.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {})
	s := &http.Server{
		Addr:        ":0",
		Handler:     server,
		ReadTimeout: cfg.ReadTimeout,
	}
	m := &Metrics{
		logger: log.With().Str("module", "metrics_test").Logger(),
		cfg:    cfg,
		s:      s,
	}
	if err := m.Start(); err != nil {
		t.Fatalf("Start() with pprof enabled failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// metrics.go — Metrics struct getter methods
// ---------------------------------------------------------------------------

func newTestMetrics(cfg config.BifrostMetricsConfiguration) *Metrics {
	s := &http.Server{
		Addr:        ":0",
		ReadTimeout: cfg.ReadTimeout,
	}
	return &Metrics{
		logger: log.With().Str("module", "metrics_test").Logger(),
		cfg:    cfg,
		s:      s,
	}
}

func TestGetCounter_Existing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	c := m.GetCounter(TotalBlockScanned)
	if c == nil {
		t.Fatal("expected non-nil counter for TotalBlockScanned")
	}
}

func TestGetCounter_Missing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	c := m.GetCounter(MetricName("does_not_exist"))
	if c != nil {
		t.Fatal("expected nil for unknown counter")
	}
}

func TestGetHistograms_Existing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	h := m.GetHistograms(BlockDiscoveryDuration)
	if h == nil {
		t.Fatal("expected non-nil histogram for BlockDiscoveryDuration")
	}
}

func TestGetHistograms_Missing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	h := m.GetHistograms(MetricName("nope"))
	if h != nil {
		t.Fatal("expected nil for unknown histogram")
	}
}

func TestGetGauge_Missing(t *testing.T) {
	// The package-level gauges map starts empty (before AddChainMetrics is called).
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	g := m.GetGauge(MetricName("nope"))
	if g != nil {
		t.Fatal("expected nil for unknown gauge")
	}
}

func TestGetCounterVec_Existing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	cv := m.GetCounterVec(CommonBlockScannerError)
	if cv == nil {
		t.Fatal("expected non-nil counter vec for CommonBlockScannerError")
	}
}

func TestGetCounterVec_Missing(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{})
	cv := m.GetCounterVec(MetricName("nope"))
	if cv != nil {
		t.Fatal("expected nil for unknown counter vec")
	}
}

func TestStart_Disabled(t *testing.T) {
	m := newTestMetrics(config.BifrostMetricsConfiguration{
		Enabled: false,
	})
	if err := m.Start(); err != nil {
		t.Fatalf("Start() with disabled config should return nil, got: %v", err)
	}
}

func TestStart_Enabled_And_Stop(t *testing.T) {
	cfg := config.BifrostMetricsConfiguration{
		Enabled:      true,
		ListenPort:   0, // use port 0 to avoid conflicts
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	m := newTestMetrics(cfg)

	if err := m.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// tss_keysign_metric.go
// ---------------------------------------------------------------------------

func TestTssKeysignMetricMgr_SetAndGet(t *testing.T) {
	mgr := NewTssKeysignMetricMgr()

	// Get non-existent key returns 0
	val := mgr.GetTssKeysignMetric("nonexistent")
	if val != 0 {
		t.Fatalf("expected 0 for missing key, got %d", val)
	}

	// Set and Get
	mgr.SetTssKeysignMetric("hash1", 42)
	val = mgr.GetTssKeysignMetric("hash1")
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}

	// Get deletes after retrieval
	val = mgr.GetTssKeysignMetric("hash1")
	if val != 0 {
		t.Fatalf("expected 0 after second Get, got %d", val)
	}
}

func TestTssKeysignMetricMgr_Overwrite(t *testing.T) {
	mgr := NewTssKeysignMetricMgr()
	mgr.SetTssKeysignMetric("h", 10)
	mgr.SetTssKeysignMetric("h", 20)

	val := mgr.GetTssKeysignMetric("h")
	if val != 20 {
		t.Fatalf("expected overwritten value 20, got %d", val)
	}
}

func TestTssKeysignMetricMgr_MultipleKeys(t *testing.T) {
	mgr := NewTssKeysignMetricMgr()
	mgr.SetTssKeysignMetric("a", 1)
	mgr.SetTssKeysignMetric("b", 2)
	mgr.SetTssKeysignMetric("c", 3)

	if v := mgr.GetTssKeysignMetric("b"); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if v := mgr.GetTssKeysignMetric("a"); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	if v := mgr.GetTssKeysignMetric("c"); v != 3 {
		t.Fatalf("expected 3, got %d", v)
	}
	// All consumed
	if v := mgr.GetTssKeysignMetric("a"); v != 0 {
		t.Fatalf("expected 0, got %d", v)
	}
}
