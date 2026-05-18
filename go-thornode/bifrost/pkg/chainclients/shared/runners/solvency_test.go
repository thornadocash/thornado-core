package runners

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type SolvencyTestSuite struct {
	sp   *DummySolvencyCheckProvider
	m    *metrics.Metrics
	cfg  config.BifrostClientConfiguration
	keys *thorclient.Keys
}

var _ = Suite(&SolvencyTestSuite{})

func (s *SolvencyTestSuite) SetUpSuite(c *C) {
	sp := &DummySolvencyCheckProvider{}
	s.sp = sp

	m, _ := metrics.NewMetrics(config.BifrostMetricsConfiguration{
		Enabled:      false,
		ListenPort:   9090,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		Chains:       common.Chains{common.ETHChain},
	})
	s.m = m

	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := ckeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(cfg.SignerName, ckeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.cfg = cfg
	s.keys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)

	c.Assert(err, IsNil)
}

func (s *SolvencyTestSuite) TestSolvencyCheck(c *C) {
	type testCase struct {
		name               string
		haltHeight         int
		solvencyHaltHeight int
		expectCheck        bool
		expectReport       bool
	}

	testCases := []testCase{
		{
			name:               "nothing halted",
			haltHeight:         0,
			solvencyHaltHeight: 0,
			expectCheck:        false,
			expectReport:       false,
		},
		{
			name:               "admin halted",
			haltHeight:         1,
			solvencyHaltHeight: 0,
			expectCheck:        false,
			expectReport:       false,
		},
		{
			name:               "future chain halt does not trigger reporting",
			haltHeight:         200,
			solvencyHaltHeight: 0,
			expectCheck:        false,
			expectReport:       false,
		},
		{
			name:               "future solvency halt does not trigger reporting",
			haltHeight:         0,
			solvencyHaltHeight: 200,
			expectCheck:        false,
			expectReport:       false,
		},
		{
			name:               "double-spend check halted chain client triggers solvency",
			haltHeight:         10,
			solvencyHaltHeight: 0,
			expectCheck:        true,
			expectReport:       true,
		},
		{
			name:               "solvency halted chain triggers reporting",
			haltHeight:         0,
			solvencyHaltHeight: 1,
			expectCheck:        true,
			expectReport:       true,
		},
	}

	for _, tc := range testCases {
		c.Log(tc.name)

		mimirMap := map[string]int{
			"HaltETHChain":         tc.haltHeight,
			"SolvencyHaltETHChain": tc.solvencyHaltHeight,
		}

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Logf("================>:%s", r.RequestURI)
			if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
				parts := strings.Split(r.RequestURI, "/key/")
				mimirKey := parts[1]
				mimirValue := 0
				if val, found := mimirMap[mimirKey]; found {
					mimirValue = val
				}
				if _, err := w.Write([]byte(strconv.Itoa(mimirValue))); err != nil {
					c.Error(err)
				}
			}
			if strings.HasPrefix(r.RequestURI, thorclient.LastBlockEndpoint) {
				if _, err := w.Write([]byte(`[{"thorchain": 100}]`)); err != nil {
					c.Error(err)
				}
			}
		})

		server := httptest.NewServer(h)
		bridge, err := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
			ChainID:         "thorchain",
			ChainHost:       server.Listener.Addr().String(),
			ChainRPC:        server.Listener.Addr().String(),
			SignerName:      "bob",
			SignerPasswd:    "password",
			ChainHomeFolder: ".",
		}, s.m, s.keys)
		c.Assert(err, IsNil)

		sp := &DummySolvencyCheckProvider{}
		sp.ResetChecks()
		stopchan := make(chan struct{})
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go SolvencyCheckRunner(common.ETHChain, sp, bridge, stopchan, wg, 10*time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		close(stopchan)
		wg.Wait()
		server.Close()

		c.Assert(sp.ShouldReportSolvencyRan(), Equals, tc.expectCheck)
		c.Assert(sp.ReportSolvencyRun(), Equals, tc.expectReport)
	}
}

// Mock SolvencyCheckProvider
type DummySolvencyCheckProvider struct {
	mu                      sync.Mutex
	shouldReportSolvencyRan bool
	reportSolvencyRun       bool
	getHeightErr            error
	shouldReport            bool
	reportSolvencyErr       error
	callCount               atomic.Int32
}

func (d *DummySolvencyCheckProvider) ResetChecks() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shouldReportSolvencyRan = false
	d.reportSolvencyRun = false
	d.getHeightErr = nil
	d.shouldReport = true
	d.reportSolvencyErr = nil
	d.callCount.Store(0)
}

func (d *DummySolvencyCheckProvider) GetHeight() (int64, error) {
	d.callCount.Add(1)
	d.mu.Lock()
	defer d.mu.Unlock()
	return 0, d.getHeightErr
}

func (d *DummySolvencyCheckProvider) ShouldReportSolvency(height int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shouldReportSolvencyRan = true
	return d.shouldReport
}

func (d *DummySolvencyCheckProvider) ReportSolvency(height int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reportSolvencyRun = true
	return d.reportSolvencyErr
}

func (d *DummySolvencyCheckProvider) ShouldReportSolvencyRan() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.shouldReportSolvencyRan
}

func (d *DummySolvencyCheckProvider) ReportSolvencyRun() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reportSolvencyRun
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerNilProvider(c *C) {
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, nil, nil, stopchan, wg, time.Millisecond*100)
	wg.Wait() // nil provider causes immediate return
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerZeroBackoff(c *C) {
	// backOffDuration == 0 should default to ThorchainBlockTime (1s in mocknet).
	// We verify this by counting calls over a known window: with a 1s interval,
	// 3.5s of sleep should yield ~3 calls. A wrong shorter fallback (e.g. 100ms)
	// would produce ~35 calls and fail the upper-bound assertion.
	mimirMap := map[string]int{
		"HaltETHChain":         10,
		"SolvencyHaltETHChain": 0,
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
			parts := strings.Split(r.RequestURI, "/key/")
			mimirKey := parts[1]
			mimirValue := 0
			if val, found := mimirMap[mimirKey]; found {
				mimirValue = val
			}
			if _, err := w.Write([]byte(strconv.Itoa(mimirValue))); err != nil {
				c.Error(err)
			}
		}
		if strings.HasPrefix(r.RequestURI, thorclient.LastBlockEndpoint) {
			if _, err := w.Write([]byte(`[{"thorchain": 100}]`)); err != nil {
				c.Error(err)
			}
		}
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	// Pass 0 as backOffDuration - should default to ThorchainBlockTime
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, 0)
	time.Sleep(time.Millisecond * 3500)
	close(stopchan)
	wg.Wait()

	calls := int(s.sp.callCount.Load())
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, true)
	// With ThorchainBlockTime=1s (mocknet), expect 2-4 calls in 3.5s.
	// A wrong sub-second default would produce far more calls.
	c.Assert(calls >= 2, Equals, true, Commentf("expected at least 2 calls, got %d", calls))
	c.Assert(calls <= 5, Equals, true, Commentf("expected at most 5 calls, got %d — interval likely shorter than ThorchainBlockTime", calls))
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerStopChannel(c *C) {
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, nil, stopchan, wg, time.Hour)
	close(stopchan)
	wg.Wait() // should return promptly via stopper
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerGetMimirError(c *C) {
	// Server that returns errors for mimir requests
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusInternalServerError)
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, time.Millisecond*200)
	time.Sleep(time.Millisecond * 500)
	close(stopchan)
	wg.Wait()
	// GetMimir errors should prevent solvency check from running
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, false)
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerGetHeightError(c *C) {
	mimirMap := map[string]int{
		"HaltETHChain":         10,
		"SolvencyHaltETHChain": 0,
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
			parts := strings.Split(r.RequestURI, "/key/")
			mimirKey := parts[1]
			mimirValue := 0
			if val, found := mimirMap[mimirKey]; found {
				mimirValue = val
			}
			if _, err := w.Write([]byte(strconv.Itoa(mimirValue))); err != nil {
				c.Error(err)
			}
		}
		if strings.HasPrefix(r.RequestURI, thorclient.LastBlockEndpoint) {
			if _, err := w.Write([]byte(`[{"thorchain": 100}]`)); err != nil {
				c.Error(err)
			}
		}
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	s.sp.getHeightErr = fmt.Errorf("height error")
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, time.Millisecond*200)
	time.Sleep(time.Millisecond * 500)
	close(stopchan)
	wg.Wait()
	// GetHeight error should prevent ShouldReportSolvency from running
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, false)
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerShouldNotReport(c *C) {
	mimirMap := map[string]int{
		"HaltETHChain":         10,
		"SolvencyHaltETHChain": 0,
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
			parts := strings.Split(r.RequestURI, "/key/")
			mimirKey := parts[1]
			mimirValue := 0
			if val, found := mimirMap[mimirKey]; found {
				mimirValue = val
			}
			if _, err := w.Write([]byte(strconv.Itoa(mimirValue))); err != nil {
				c.Error(err)
			}
		}
		if strings.HasPrefix(r.RequestURI, thorclient.LastBlockEndpoint) {
			if _, err := w.Write([]byte(`[{"thorchain": 100}]`)); err != nil {
				c.Error(err)
			}
		}
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	s.sp.shouldReport = false
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, time.Millisecond*200)
	time.Sleep(time.Millisecond * 500)
	close(stopchan)
	wg.Wait()
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, true)
	c.Assert(s.sp.ReportSolvencyRun(), Equals, false)
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerReportError(c *C) {
	mimirMap := map[string]int{
		"HaltETHChain":         10,
		"SolvencyHaltETHChain": 0,
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
			parts := strings.Split(r.RequestURI, "/key/")
			mimirKey := parts[1]
			mimirValue := 0
			if val, found := mimirMap[mimirKey]; found {
				mimirValue = val
			}
			if _, err := w.Write([]byte(strconv.Itoa(mimirValue))); err != nil {
				c.Error(err)
			}
		}
		if strings.HasPrefix(r.RequestURI, thorclient.LastBlockEndpoint) {
			if _, err := w.Write([]byte(`[{"thorchain": 100}]`)); err != nil {
				c.Error(err)
			}
		}
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	s.sp.reportSolvencyErr = fmt.Errorf("report failed")
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, time.Millisecond*200)
	time.Sleep(time.Millisecond * 500)
	close(stopchan)
	wg.Wait()
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, true)
	c.Assert(s.sp.ReportSolvencyRun(), Equals, true) // it ran, but returned error
}

func (s *SolvencyTestSuite) TestSolvencyCheckRunnerSolvencyHaltError(c *C) {
	// First mimir call succeeds (HaltChain), second fails (SolvencyHalt)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint) {
			parts := strings.Split(r.RequestURI, "/key/")
			mimirKey := parts[1]
			if strings.HasPrefix(mimirKey, "SolvencyHalt") {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			// HaltETHChain returns 0 (not halted)
			if _, err := w.Write([]byte("0")); err != nil {
				c.Error(err)
			}
		}
	})
	server := httptest.NewServer(h)
	defer server.Close()
	bridge, _ := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, s.m, s.keys)

	s.sp.ResetChecks()
	stopchan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go SolvencyCheckRunner(common.ETHChain, s.sp, bridge, stopchan, wg, time.Millisecond*200)
	time.Sleep(time.Millisecond * 500)
	close(stopchan)
	wg.Wait()
	// Solvency halt error should prevent solvency check
	c.Assert(s.sp.ShouldReportSolvencyRan(), Equals, false)
}

func (s *SolvencyTestSuite) TestIsVaultSolvent(c *C) {
	vault := types.Vault{
		BlockHeight: 1,
		PubKey:      types.GetRandomPubKey(),
		Coins: common.NewCoins(
			common.NewCoin(common.ETHAsset, cosmos.NewUint(102400000000)),
		),
		Type:   types.VaultType_AsgardVault,
		Status: types.VaultStatus_ActiveVault,
	}
	acct := common.Account{
		Sequence:      0,
		AccountNumber: 0,
		Coins:         common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(102400000000))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.NewUint(0)), Equals, true)
	acct = common.Account{
		Sequence:      0,
		AccountNumber: 0,
		Coins:         common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(102305000000))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.NewUint(80000*120)), Equals, true)
	acct = common.Account{
		Sequence:      0,
		AccountNumber: 0,
		Coins:         common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(102205000000))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.NewUint(80000*120)), Equals, false)

	tokenAsset, err := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(err, IsNil)
	vault.Coins = common.NewCoins(
		common.NewCoin(common.ETHAsset, cosmos.NewUint(102400000000)),
		common.NewCoin(tokenAsset, cosmos.NewUint(100)),
	)
	acct = common.Account{
		Sequence:      0,
		AccountNumber: 0,
		Coins:         common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(102400000000))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.NewUint(0)), Equals, false)

	vault.Coins = common.NewCoins(
		common.NewCoin(common.ETHAsset, cosmos.NewUint(102400000000)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100)),
	)
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.NewUint(0)), Equals, true)

	// Non-gas asset insolvency (no gas tolerance applies)
	usdtAsset, err := common.NewAsset("ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7")
	c.Assert(err, IsNil)
	vault2 := types.Vault{
		BlockHeight: 1,
		PubKey:      types.GetRandomPubKey(),
		Coins: common.NewCoins(
			common.NewCoin(usdtAsset, cosmos.NewUint(1000)),
		),
		Type:   types.VaultType_AsgardVault,
		Status: types.VaultStatus_ActiveVault,
	}
	// Wallet has less than vault for non-gas asset -> insolvent
	acct = common.Account{
		Coins: common.NewCoins(common.NewCoin(usdtAsset, cosmos.NewUint(999))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault2, cosmos.NewUint(0)), Equals, false)

	// Wallet has more than vault for non-gas asset -> solvent
	acct = common.Account{
		Coins: common.NewCoins(common.NewCoin(usdtAsset, cosmos.NewUint(1001))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault2, cosmos.NewUint(0)), Equals, true)

	// Wallet has exactly same as vault -> solvent
	acct = common.Account{
		Coins: common.NewCoins(common.NewCoin(usdtAsset, cosmos.NewUint(1000))),
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault2, cosmos.NewUint(0)), Equals, true)

	// Empty account coins with non-zero vault -> insolvent
	acct = common.Account{
		Coins: common.Coins{},
	}
	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault2, cosmos.NewUint(0)), Equals, false)
}

func (s *SolvencyTestSuite) TestIsVaultSolventMissingZeroAmountAssetDoesNotLogInsolvent(c *C) {
	originalLogger := log.Logger
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)
	defer func() {
		log.Logger = originalLogger
	}()

	tokenAsset, err := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(err, IsNil)

	vault := types.Vault{
		BlockHeight: 1,
		PubKey:      types.GetRandomPubKey(),
		Coins: common.NewCoins(
			common.NewCoin(tokenAsset, cosmos.ZeroUint()),
		),
		Type:   types.VaultType_AsgardVault,
		Status: types.VaultStatus_ActiveVault,
	}
	acct := common.Account{
		Sequence:      0,
		AccountNumber: 0,
		Coins:         common.NewCoins(),
	}

	c.Assert(IsVaultSolvent(common.ETHChain, acct, vault, cosmos.ZeroUint()), Equals, true)
	c.Assert(strings.Contains(buf.String(), "insolvent"), Equals, false)
}
