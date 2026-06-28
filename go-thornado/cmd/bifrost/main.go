package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	golog "github.com/ipfs/go-log"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/p2p"

	"github.com/thornadocash/go-thornado/app"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/observer"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc"
	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/signer"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/cmd"
	tcommon "github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
)

const (
	serverIdentity = "bifrost"
)

func printVersion() {
	fmt.Printf("%s v%s, rev %s\n", serverIdentity, constants.Version, constants.GitCommit)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "observe-tx" {
		runObserveTxCommand(os.Args[2:])
		return
	}

	showVersion := flag.Bool("version", false, "Shows version")
	logLevel := flag.StringP("log-level", "l", "info", "Log Level")
	pretty := flag.BoolP("pretty-log", "p", false, "Enables unstructured prettified logging. This is useful for local debugging")
	deckDump := flag.String("deck-dump", "", "Path to a deck dump file")
	flag.Parse()

	if *showVersion {
		printVersion()
		return
	}

	initPrefix()
	initLog(*logLevel, *pretty)
	config.Init()
	config.InitBifrost()
	cfg := config.GetBifrost()

	// metrics
	m, err := metrics.NewMetrics(cfg.Metrics)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create metric instance")
	}
	if err = m.Start(); err != nil {
		log.Fatal().Err(err).Msg("fail to start metric collector")
	}
	if len(cfg.Thornado.SignerName) == 0 {
		log.Fatal().Msg("signer name is empty")
	}
	if len(cfg.Thornado.SignerPasswd) == 0 {
		log.Fatal().Msg("signer password is empty")
	}
	kb, _, err := thornadoclient.GetKeyringKeybase(cfg.Thornado.ChainHomeFolder, cfg.Thornado.SignerName, cfg.Thornado.SignerPasswd)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to get keyring keybase")
	}

	k := thornadoclient.NewKeysWithKeybase(kb, cfg.Thornado.SignerName, cfg.Thornado.SignerPasswd)
	// thornado bridge
	thornadoBridge, err := thornadoclient.NewThornadoBridge(cfg.Thornado, m, k)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create new thornado bridge")
	}
	if err = thornadoBridge.EnsureNodeWhitelistedWithTimeout(); err != nil {
		log.Fatal().Err(err).Msg("node account is not whitelisted, can't start")
	}
	// PubKey Manager
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(thornadoBridge, m)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create pubkey manager")
	}
	if err = pubkeyMgr.Start(); err != nil {
		log.Fatal().Err(err).Msg("fail to start pubkey manager")
	}

	// setup FROST signing transport identity
	priKey, err := k.GetPrivateKey()
	if err != nil {
		log.Fatal().Err(err).Msg("fail to get secp256k1 private key")
	}

	tmPrivateKey := tcommon.CosmosPrivateKeyToTMPrivateKey(priKey)

	consts := constants.NewConfigValue()
	jailTimeKeygen := time.Duration(consts.GetInt64Value(constants.Keygen_FailJailMinutes)) * time.Minute
	jailTimeKeysign := time.Duration(consts.GetInt64Value(constants.Keysign_FailJailMinutes)) * time.Minute
	if cfg.Signer.KeygenTimeout >= jailTimeKeygen {
		log.Fatal().
			Stringer("keygenTimeout", cfg.Signer.KeygenTimeout).
			Stringer("keygenJail", jailTimeKeygen).
			Msg("keygen timeout must be shorter than jail time")
	}
	if cfg.Signer.KeysignTimeout >= jailTimeKeysign {
		log.Fatal().
			Stringer("keysignTimeout", cfg.Signer.KeysignTimeout).
			Stringer("keysignJail", jailTimeKeysign).
			Msg("keysign timeout must be shorter than jail time")
	}

	localStateFolder := filepath.Dir(cfg.Signer.SignerDbPath)
	if localStateFolder == "." || localStateFolder == "" {
		localStateFolder = app.DefaultNodeHome
	}

	// Start P2P with bonded node gating enabled
	comm, stateManager, err := p2p.StartP2PWithBridge(
		cfg.FROST,
		tmPrivateKey,
		localStateFolder,
		thornadoBridge,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to start p2p")
	}

	cfgChains := cfg.GetChains()

	// ensure we have a protocol for chain RPC Hosts
	for _, chainCfg := range cfgChains {
		if chainCfg.Disabled {
			continue
		}
		if len(chainCfg.RPCHost) == 0 {
			log.Fatal().Err(err).Stringer("chain", chainCfg.ChainID).Msg("missing chain RPC host")
			return
		}
		if !strings.HasPrefix(chainCfg.RPCHost, "http") {
			chainCfg.RPCHost = fmt.Sprintf("http://%s", chainCfg.RPCHost)
		}
	}
	partyCoordinator := p2p.NewPartyCoordinator(comm.GetHost(), cfg.Signer.PartyTimeout)
	defer partyCoordinator.Stop()
	sessionCoordinator := frost.NewP2PSessionCoordinator(
		comm,
		partyCoordinator,
		cfg.Signer.KeygenTimeout,
		cfg.Signer.KeysignTimeout,
	)
	sessionCoordinator.SetLogger(log.With().Str("module", "frost_p2p").Logger())

	chains, restart := chainclients.LoadChains(k, cfgChains, thornadoBridge, stateManager, m, pubkeyMgr, sessionCoordinator)
	if len(chains) == 0 {
		log.Fatal().Msg("fail to load any chains")
	}
	frostKeysignMetricMgr := metrics.NewFrostKeysignMetricMgr()
	healthServer := NewHealthServer(cfg.FROST.InfoAddress, comm.GetHost().ID().String(), chains)
	healthServer.SetFrostSessionDebugger(sessionCoordinator)
	go func() {
		defer log.Info().Msg("health server exit")
		if err = healthServer.Start(); err != nil {
			log.Error().Err(err).Msg("fail to start health server")
		}
	}()

	ctx := context.Background()

	// start observer notifier
	ag, err := observer.NewAttestationGossip(comm.GetHost(), k, cfg.Thornado.ChainEBifrost, thornadoBridge, m, cfg.AttestationGossip)
	healthServer.SetAttestationDebugger(ag)

	// start observer
	obs, err := observer.NewObserver(pubkeyMgr, chains, thornadoBridge, m, cfgChains[tcommon.BTCChain].BlockScanner.DBPath, frostKeysignMetricMgr, ag, *deckDump)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create observer")
	}
	healthServer.SetObserverDebugger(obs)
	if err = obs.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("fail to start observer")
	}

	// enable observer to react to notifications from thornado
	// that come through the grpc connection within AttestationGossip.
	ag.SetObserverHandleObservedTxCommitted(obs)

	// start signer
	sign, err := signer.NewSigner(cfg, thornadoBridge, k, stateManager, pubkeyMgr, chains, m, frostKeysignMetricMgr, obs, sessionCoordinator)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create instance of signer")
	}
	healthServer.SetSigner(sign)
	if err = sign.Start(); err != nil {
		log.Fatal().Err(err).Msg("fail to start signer")
	}

	// wait....
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-ch:
	case <-restart:
	}
	log.Info().Msg("stop signal received")

	// stop observer
	if err = obs.Stop(); err != nil {
		log.Fatal().Err(err).Msg("fail to stop observer")
	}
	// stop signer
	if err = sign.Stop(); err != nil {
		log.Fatal().Err(err).Msg("fail to stop signer")
	}
	if err = healthServer.Stop(); err != nil {
		log.Fatal().Err(err).Msg("fail to stop health server")
	}
}

func runObserveTxCommand(args []string) {
	fs := flag.NewFlagSet("observe-tx", flag.ExitOnError)
	logLevel := fs.StringP("log-level", "l", "info", "Log Level")
	pretty := fs.BoolP("pretty-log", "p", false, "Enables unstructured prettified logging")
	chainName := fs.String("chain", tcommon.BTCChain.String(), "Chain to observe")
	allowFutureObservation := fs.Bool("allow-future-observation", false, "Submit as future observation for instant signed outbound recovery")
	if err := fs.Parse(args); err != nil {
		log.Fatal().Err(err).Msg("fail to parse observe-tx flags")
	}
	txids := fs.Args()
	if len(txids) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bifrost observe-tx [--chain BTC] <txid> [txid...]")
		os.Exit(2)
	}

	initPrefix()
	initLog(*logLevel, *pretty)
	config.Init()
	config.InitBifrost()
	cfg := config.GetBifrost()

	m, err := metrics.NewMetrics(cfg.Metrics)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create metric instance")
	}
	if len(cfg.Thornado.SignerName) == 0 {
		log.Fatal().Msg("signer name is empty")
	}
	if len(cfg.Thornado.SignerPasswd) == 0 {
		log.Fatal().Msg("signer password is empty")
	}
	kb, _, err := thornadoclient.GetKeyringKeybase(cfg.Thornado.ChainHomeFolder, cfg.Thornado.SignerName, cfg.Thornado.SignerPasswd)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to get keyring keybase")
	}
	k := thornadoclient.NewKeysWithKeybase(kb, cfg.Thornado.SignerName, cfg.Thornado.SignerPasswd)
	thornadoBridge, err := thornadoclient.NewThornadoBridge(cfg.Thornado, m, k)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create new thornado bridge")
	}
	if err = thornadoBridge.EnsureNodeWhitelistedWithTimeout(); err != nil {
		log.Fatal().Err(err).Msg("node account is not whitelisted")
	}
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(thornadoBridge, m)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create pubkey manager")
	}
	if err = pubkeyMgr.Start(); err != nil {
		log.Fatal().Err(err).Msg("fail to start pubkey manager")
	}
	defer pubkeyMgr.Stop()

	chainID := tcommon.Chain(*chainName)
	if !chainID.Equals(tcommon.BTCChain) {
		log.Fatal().Stringer("chain", chainID).Msg("observe-tx only supports BTC")
	}
	chainCfg := cfg.GetChains()[chainID]
	if chainCfg.Disabled {
		log.Fatal().Stringer("chain", chainID).Msg("chain is disabled")
	}
	if len(chainCfg.RPCHost) == 0 {
		log.Fatal().Stringer("chain", chainID).Msg("missing chain RPC host")
	}
	if !strings.HasPrefix(chainCfg.RPCHost, "http") {
		chainCfg.RPCHost = fmt.Sprintf("http://%s", chainCfg.RPCHost)
	}
	chainClient, err := btc.NewObserveOnlyClient(k, chainCfg, thornadoBridge, m)
	if err != nil {
		log.Fatal().Err(err).Msg("fail to create observe-only BTC client")
	}
	results, err := observer.ManualObserveTxIDs(
		context.Background(),
		k,
		cfg.Thornado.ChainEBifrost,
		thornadoBridge,
		pubkeyMgr,
		chainClient,
		txids,
		*allowFutureObservation,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("manual observe failed")
	}
	for _, result := range results {
		log.Info().
			Str("txid", result.TxID).
			Stringer("chain", result.Chain).
			Bool("mempool", result.Mempool).
			Bool("finalized", result.Finalized).
			Int("inbound", result.Inbound).
			Int("outbound", result.Outbound).
			Int64("confirmations_required", result.RequiredConfirmations).
			Int64("finalise_height", result.ObservationFinaliseHeight).
			Bool("allow_future_observation", result.AllowFutureObservation).
			Msg("manual observe submitted")
	}
}

func initPrefix() {
	cosmosSDKConfg := cosmos.GetConfig()
	cosmosSDKConfg.SetBech32PrefixForAccount(cmd.Bech32PrefixAccAddr, cmd.Bech32PrefixAccPub)
	cosmosSDKConfg.Seal()
}

func initLog(level string, pretty bool) {
	l, err := zerolog.ParseLevel(level)
	if err != nil {
		log.Warn().Msgf("%s is not a valid log-level, falling back to 'info'", level)
	}
	var out io.Writer = os.Stdout
	if pretty {
		out = zerolog.ConsoleWriter{Out: os.Stdout}
	}
	zerolog.SetGlobalLevel(l)
	log.Logger = log.Output(out).With().Caller().Str("service", serverIdentity).Logger()

	logLevel := golog.LevelInfo
	switch l {
	case zerolog.DebugLevel:
		logLevel = golog.LevelDebug
	case zerolog.InfoLevel:
		logLevel = golog.LevelInfo
	case zerolog.ErrorLevel:
		logLevel = golog.LevelError
	case zerolog.FatalLevel:
		logLevel = golog.LevelFatal
	case zerolog.PanicLevel:
		logLevel = golog.LevelPanic
	}
	golog.SetAllLoggers(logLevel)
}
