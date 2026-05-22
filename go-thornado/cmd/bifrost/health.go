package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	openapi "github.com/thornadocash/go-thornado/openapi/gen"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// -------------------------------------------------------------------------------------
// Responses
// -------------------------------------------------------------------------------------

type P2PStatusPeer struct {
	Address string `json:"address"`
	IP      string `json:"ip"`
	Status  string `json:"status"`

	StoredPeerID   string `json:"stored_peer_id"`
	NodesPeerID    string `json:"nodes_peer_id"`
	ReturnedPeerID string `json:"returned_peer_id"`

	P2PPortOpen bool  `json:"p2p_port_open"`
	P2PDialMs   int64 `json:"p2p_dial_ms"`
}

type P2PStatusResponse struct {
	ThornadoHeight int64           `json:"thornado_height"`
	Peers          []P2PStatusPeer `json:"peers"`
	PeerCount      int             `json:"peer_count"`
	Errors         []string        `json:"errors"`
}

type ProviderEntry struct {
	RPCHost   string `json:"rpc_host,omitempty"`
	APIHost   string `json:"api_host,omitempty"`
	ChainHost string `json:"chain_host,omitempty"`
	GRPCHost  string `json:"grpc_host,omitempty"`
}

type ProviderResponse map[string]ProviderEntry

type ScannerResponse struct {
	Chain              string `json:"chain"`
	ChainHeight        int64  `json:"chain_height"`
	BlockScannerHeight int64  `json:"block_scanner_height"`
	ScannerHeightDiff  int64  `json:"scanner_height_diff"`
	Healthy            *bool  `json:"healthy,omitempty"`
}

type signingChain struct {
	Chain               string `json:"chain"`
	LatestBroadcastedTx string `json:"latest_broadcasted_tx"`
	LatestObservedTx    string `json:"latest_observed_tx"`
	CurrentSequence     int64  `json:"current_sequence"`
}

type VaultResponse struct {
	Pubkey       common.PubKey     `json:"pubkey"`
	Status       types.VaultStatus `json:"status"`
	ChainDetails []signingChain    `json:"chain_details"`
}

// -------------------------------------------------------------------------------------
// Health Server
// -------------------------------------------------------------------------------------

// HealthServer to provide something for health check and also p2pid
type HealthServer struct {
	logger          zerolog.Logger
	s               *http.Server
	localPeerID     string
	chains          map[common.Chain]chainclients.ChainClient
	providerPayload []byte
}

// NewHealthServer create a new instance of health server
func NewHealthServer(addr string, localPeerID string, chains map[common.Chain]chainclients.ChainClient) *HealthServer {
	res := make(ProviderResponse)
	for chain, client := range chains {
		cfg := client.GetConfig()
		res[chain.String()] = ProviderEntry{
			RPCHost:   classifyHost(cfg.RPCHost),
			APIHost:   classifyHost(cfg.APIHost),
			ChainHost: classifyHost(cfg.ChainHost),
			GRPCHost:  classifyHost(cfg.CosmosGRPCHost),
		}
	}
	providerPayload, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("fail to marshal provider status")
	}

	hs := &HealthServer{
		logger:          log.With().Str("module", "http").Logger(),
		localPeerID:     localPeerID,
		chains:          chains,
		providerPayload: providerPayload,
	}
	s := &http.Server{
		Addr:              addr,
		Handler:           hs.newHandler(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	hs.s = s

	return hs
}

func (s *HealthServer) newHandler() http.Handler {
	router := mux.NewRouter()
	router.Handle("/ping", http.HandlerFunc(s.pingHandler)).Methods(http.MethodGet)
	router.Handle("/p2pid", http.HandlerFunc(s.getP2pIDHandler)).Methods(http.MethodGet)
	router.Handle("/status/p2p", http.HandlerFunc(s.p2pStatus)).Methods(http.MethodGet)
	router.Handle("/status/scanner", http.HandlerFunc(s.chainScanner)).Methods(http.MethodGet)
	router.Handle("/status/provider", http.HandlerFunc(s.providerStatus)).Methods(http.MethodGet)
	router.Handle("/status/signing", http.HandlerFunc(s.currentSigning)).Methods(http.MethodGet)
	router.Handle("/version", http.HandlerFunc(s.versionHandler)).Methods(http.MethodGet)
	return router
}

func (s *HealthServer) pingHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *HealthServer) versionHandler(w http.ResponseWriter, _ *http.Request) {
	resp := struct {
		Version   string `json:"version"`
		GitCommit string `json:"git_commit"`
	}{
		Version:   constants.Version,
		GitCommit: constants.GitCommit,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error().Err(err).Msg("fail to write version response")
	}
}

func (s *HealthServer) getP2pIDHandler(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte(s.localPeerID))
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to write to response")
	}
}

func (s *HealthServer) p2pStatus(w http.ResponseWriter, _ *http.Request) {
	res := &P2PStatusResponse{Peers: make([]P2PStatusPeer, 0)}

	// get thornado nodes
	nodesByIP := map[string]openapi.Node{}
	thornado := config.GetBifrost().Thornado.ChainHost
	url := fmt.Sprintf("http://%s/thornado/nodes", thornado)
	resp, err := http.Get(url)
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get thornado status")
	} else {
		defer resp.Body.Close()

		// set the height from header
		res.ThornadoHeight, err = strconv.ParseInt(resp.Header.Get("grpc-metadata-x-cosmos-block-height"), 10, 64)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to parse thornado height")
		}

		nodes := make([]openapi.Node, 0)
		if err = json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
			s.logger.Error().Err(err).Msg("fail to decode thornado status")
		} else {
			for _, node := range nodes {
				otherNode, exists := nodesByIP[node.IpAddress]

				if !exists || (otherNode.Status != types.NodeStatus_Active.String() && otherNode.PreflightStatus.Status != "Ready") {
					// only add node if the IP is not already in the map
					nodesByIP[node.IpAddress] = node
				} else if node.Status == types.NodeStatus_Active.String() || node.PreflightStatus.Status == "Ready" {
					// if both nodes are active or ready, report an error
					res.Errors = append(res.Errors, fmt.Sprintf("active node IP reuse: %s", node.IpAddress))
				}
			}
		}
	}

	res.PeerCount = 0

	// write the response
	jsonBytes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to write to response")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		_, err = w.Write(jsonBytes)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to write to response")
		}
	}
}

func (s *HealthServer) currentSigning(w http.ResponseWriter, _ *http.Request) {
	res := make([]VaultResponse, 0)

	thornado := config.GetBifrost().Thornado.ChainHost
	url := fmt.Sprintf("http://%s%s", thornado, thornadoclient.AsgardVault)
	resp, err := http.Get(url)
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get thornado status")
	} else {
		defer resp.Body.Close()

		vaults := make([]openapi.Vault, 0)
		if err = json.NewDecoder(resp.Body).Decode(&vaults); err != nil {
			s.logger.Error().Err(err).Msg("fail to decode thornado status")
		}
		for _, vault := range vaults {
			valRes := VaultResponse{
				Pubkey:       common.PubKey(*vault.PubKey),
				Status:       types.VaultStatus(types.VaultStatus_value[vault.Status]),
				ChainDetails: make([]signingChain, 0),
			}

			for _, chain := range vault.Chains {
				client, found := s.chains[common.Chain(strings.ToUpper(chain))]
				if !found {
					s.logger.Error().Msgf("failed to get bifrost chain client for %s", chain)
					continue
				}
				var account common.Account
				account, err = client.GetAccount(common.PubKey(*vault.PubKey), nil)
				if err != nil {
					s.logger.Error().Err(err).Msgf("failed to get account for vault:%s on chain:%s", *vault.PubKey, chain)
					continue
				}
				var lastObserved, lastBroadcasted string
				lastObserved, lastBroadcasted, err = client.GetLatestTxForVault(*vault.PubKey)
				if err != nil {
					s.logger.Error().Err(err).Msgf("failed to get latest tx for vault:%s on chain:%s", *vault.PubKey, chain)
				}
				valRes.ChainDetails = append(valRes.ChainDetails, signingChain{
					Chain:               chain,
					LatestBroadcastedTx: lastBroadcasted,
					LatestObservedTx:    lastObserved,
					CurrentSequence:     account.Sequence,
				})
			}
			res = append(res, valRes)
		}
	}

	// write the response
	jsonBytes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to write to response")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		_, err = w.Write(jsonBytes)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to write to response")
		}
	}
}

func (s *HealthServer) chainScanner(w http.ResponseWriter, _ *http.Request) {
	res := make(map[string]ScannerResponse)

	// Iterate through each chain client
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	for chain, client := range s.chains {
		wg.Add(1)
		chain := chain
		client := client
		go func() {
			defer wg.Done()

			// Fetch the current block height of the chain daemon
			height, err := client.GetHeight()
			if err != nil {
				// failed to get chain height
				height = -1
			}

			// check for local blockScanner height
			blockScannerHeight, err := client.GetBlockScannerHeight()
			if err != nil {
				blockScannerHeight = -1
			}

			var scannerHeightDiff int64
			if height < 0 || blockScannerHeight < 0 {
				scannerHeightDiff = -1
			} else {
				scannerHeightDiff = height - blockScannerHeight
			}

			healthy := client.IsBlockScannerHealthy()
			mu.Lock()
			res[chain.String()] = ScannerResponse{
				Chain:              chain.String(),
				ChainHeight:        height,
				BlockScannerHeight: blockScannerHeight,
				ScannerHeightDiff:  scannerHeightDiff,
				Healthy:            &healthy,
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Fetch thornado height
	thornado := config.GetBifrost().Thornado.ChainHost
	url := fmt.Sprintf("http://%s/thornado/lastblock", thornado)
	resp, err := http.Get(url)
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get thornado status")
	} else {
		defer resp.Body.Close()
		var height int64
		height, err = strconv.ParseInt(resp.Header.Get("grpc-metadata-x-cosmos-block-height"), 10, 64)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to parse thornado height")
		}
		res[common.Thornado.String()] = ScannerResponse{
			Chain:              common.Thornado.String(),
			ChainHeight:        height,
			BlockScannerHeight: -1, // TODO: pending for thornado
			ScannerHeightDiff:  -1,
		}
	}

	// write the response
	jsonBytes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to write to response")
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		_, err = w.Write(jsonBytes)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to write to response")
		}
	}
}

func (s *HealthServer) providerStatus(w http.ResponseWriter, _ *http.Request) {
	if s.providerPayload == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(s.providerPayload); err != nil {
		s.logger.Error().Err(err).Msg("fail to write to response")
	}
}

// classifyHost returns "self-hosted" when rawURL resolves to a loopback, private,
// link-local, or bare (no-dot) hostname, and returns the registrable domain
// (e.g. "infura.io") for external providers. An empty input returns "".
func classifyHost(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Add a scheme so url.Parse can extract the hostname reliably.
	toParse := rawURL
	if !strings.Contains(toParse, "://") {
		toParse = "http://" + toParse
	}
	u, err := url.Parse(toParse)
	if err != nil {
		return "self-hosted"
	}
	host := u.Hostname()
	if host == "" {
		return "self-hosted"
	}

	// IP address – check whether it is in a local/private range.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return "self-hosted"
		}
		return "external-host" // public IP
	}

	// Bare hostnames (no dots) such as "localhost", "bitcoin-daemon", "thornado".
	if strings.EqualFold(host, "localhost") || !strings.Contains(host, ".") {
		return "self-hosted"
	}

	// Local-network TLDs.
	lower := strings.ToLower(host)
	for _, suffix := range []string{".local", ".internal", ".lan", ".home", ".localdomain"} {
		if strings.HasSuffix(lower, suffix) {
			return "self-hosted"
		}
	}

	// External provider – return the registrable domain (last two labels).
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return "unknown"
}

// Start health server
func (s *HealthServer) Start() error {
	if s.s == nil {
		return errors.New("invalid http server instance")
	}
	if err := s.s.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			return fmt.Errorf("fail to start http server: %w", err)
		}
	}
	return nil
}

func (s *HealthServer) Stop() error {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := s.s.Shutdown(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to shutdown the Tss server gracefully")
	}
	return err
}

func checkPortOpen(host string, port int) bool {
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
