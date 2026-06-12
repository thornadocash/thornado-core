package thornadoclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/app"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/constants"
	openapi "github.com/thornadocash/go-thornado/openapi/gen"
	stypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

// Endpoint urls
const (
	AuthAccountEndpoint      = "/cosmos/auth/v1beta1/accounts"
	BroadcastTxsEndpoint     = "/"
	KeygenEndpoint           = "/thornado/keygen"
	KeysignEndpoint          = "/thornado/keysign"
	LastBlockEndpoint        = "/thornado/lastblock"
	NodeAccountEndpoint      = "/thornado/node"
	NodeAccountsEndpoint     = "/thornado/nodes"
	SignerMembershipEndpoint = "/thornado/vaults/%s/signers"
	StatusEndpoint           = "/status"
	VaultEndpoint            = "/thornado/vault/%s"
	BaseVaultEndpoint        = "/thornado/vaults/base"
	PubKeysEndpoint          = "/thornado/vaults/pubkeys"
	ConfigEndpoint           = "/thornado/config"
	ConfigDefaultsEndpoint   = "/thornado/config/defaults"
	ChainVersionEndpoint     = "/thornado/version"
	NetworkFeeEndpoint       = "/thornado/network_fee"
)

// thornadoBridge will be used to send tx to Thornado
type thornadoBridge struct {
	logger        zerolog.Logger
	cfg           config.BifrostClientConfiguration
	keys          *Keys
	errCounter    *prometheus.CounterVec
	m             *metrics.Metrics
	blockHeight   int64
	accountNumber uint64
	seqNumber     uint64
	httpClient    *retryablehttp.Client
	broadcastLock *sync.RWMutex
}

type ThornadoBridge interface {
	EnsureNodeWhitelisted() error
	EnsureNodeWhitelistedWithTimeout() error
	FetchNodeStatus() (stypes.NodeStatus, error)
	FetchActiveNodes() ([]common.PubKey, error)
	GetBaseVaults() (stypes.Vaults, error)
	GetVault(pubkey string) (stypes.Vault, error)
	GetConfig() config.BifrostClientConfiguration
	GetConstants() (map[string]int64, error)
	GetContext() client.Context
	GetErrataMsg(txID common.TxID, chain common.Chain) sdk.Msg
	GetKeygenStdTx(vaultPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []stypes.Blame, inputPks common.PubKeys, keygenType stypes.KeygenType, chains common.Chains, height, keygenTime int64, vaultPubKeyEddsa common.PubKey, keysharesBackupEddsa []byte) (sdk.Msg, error)
	GetKeysignParty(vaultPubKey common.PubKey) (common.PubKeys, error)
	GetConfigValue(key string) (int64, error)
	GetConfigValueWithRef(template, ref string) (int64, error)
	GetInboundOutbound(txIns common.ObservedTxs) (common.ObservedTxs, common.ObservedTxs, error)
	GetPubKeys() ([]PubKeyAddressPair, error)
	GetBasePubKeys() ([]PubKeyAddressPair, error)
	GetSolvencyMsg(height int64, chain common.Chain, pubKey common.PubKey, coins common.Coins) *stypes.MsgSolvency
	GetThornadoVersion() (semver.Version, error)
	IsCatchingUp() (bool, error)
	HasNetworkFee(chain common.Chain) (bool, error)
	GetNetworkFee(chain common.Chain) (transactionSize, transactionFeeRate uint64, err error)
	PostKeysignFailure(blame stypes.Blame, height int64, coins common.Coins, pubkey common.PubKey) (common.TxID, error)
	PostNetworkFee(height int64, chain common.Chain, transactionSize, transactionRate uint64) (common.TxID, error)
	WaitToCatchUp() error
	GetBlockHeight() (int64, error)
	GetBlockTimestamp(height int64) (time.Time, error)
	GetLastObservedInHeight(chain common.Chain) (int64, error)
	GetLastSignedOutHeight(chain common.Chain) (int64, error)
	Broadcast(msgs ...sdk.Msg) (common.TxID, error)
	BroadcastWithBlocking(msgs ...sdk.Msg) (common.TxID, error)
	GetKeysign(blockHeight int64, pk string) (types.TxOut, error)
	GetNodeAccount(string) (*stypes.NodeAccount, error)
	GetNodeAccounts() ([]*stypes.QueryNodeResponse, error)
	GetKeygenBlock(int64, string) (stypes.KeygenBlock, error)
}

// httpResponseCache used for caching HTTP responses for less frequent querying
type httpResponseCache struct {
	httpResponse        []byte
	httpResponseChecked time.Time
	httpResponseMu      *sync.Mutex
}

var (
	httpResponseCaches   = make(map[string]*httpResponseCache) // String-to-pointer map for quicker lookup
	httpResponseCachesMu = &sync.Mutex{}
)

// NewThornadoBridge create a new instance of ThornadoBridge
func NewThornadoBridge(cfg config.BifrostClientConfiguration, m *metrics.Metrics, k *Keys) (ThornadoBridge, error) {
	// main module logger
	logger := log.With().Str("module", "thornado_client").Logger()

	if len(cfg.ChainID) == 0 {
		return nil, errors.New("chain id is empty")
	}
	if len(cfg.ChainHost) == 0 {
		return nil, errors.New("chain host is empty")
	}

	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil

	return &thornadoBridge{
		logger:        logger,
		cfg:           cfg,
		keys:          k,
		errCounter:    m.GetCounterVec(metrics.ThornadoClientError),
		httpClient:    httpClient,
		m:             m,
		broadcastLock: &sync.RWMutex{},
	}, nil
}

func MakeCodec() codec.ProtoCodecMarshaler {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	stypes.RegisterInterfaces(interfaceRegistry)
	return codec.NewProtoCodec(interfaceRegistry)
}

// GetContext return a valid context with all relevant values set
func (b *thornadoBridge) GetContext() client.Context {
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		panic(err)
	}
	ctx := client.Context{}
	ctx = ctx.WithKeyring(b.keys.GetKeybase())
	ctx = ctx.WithChainID(string(b.cfg.ChainID))
	ctx = ctx.WithHomeDir(b.cfg.ChainHomeFolder)
	ctx = ctx.WithFromName(b.cfg.SignerName)
	ctx = ctx.WithFromAddress(signerAddr)
	ctx = ctx.WithBroadcastMode("sync")

	encodingConfig := app.MakeEncodingConfig()
	ctx = ctx.WithCodec(encodingConfig.Codec)
	ctx = ctx.WithInterfaceRegistry(encodingConfig.InterfaceRegistry)
	ctx = ctx.WithTxConfig(encodingConfig.TxConfig)
	ctx = ctx.WithLegacyAmino(encodingConfig.Amino)
	ctx = ctx.WithAccountRetriever(authtypes.AccountRetriever{})

	remote := b.cfg.ChainRPC
	if !strings.HasPrefix(b.cfg.ChainHost, "http") {
		remote = fmt.Sprintf("tcp://%s", remote)
	}
	ctx = ctx.WithNodeURI(remote)
	client, err := rpchttp.New(remote, "/websocket")
	if err != nil {
		panic(err)
	}
	ctx = ctx.WithClient(client)
	return ctx
}

func (b *thornadoBridge) getWithPath(path string) ([]byte, int, error) {
	return b.get(b.getThorChainURL(path))
}

// get handle all the low level http GET calls using retryablehttp.ThornadoBridge
func (b *thornadoBridge) get(url string) ([]byte, int, error) {
	// To reduce querying time and chance of "429 Too Many Requests",
	// do not query the same endpoint more than once per block time.
	httpResponseCachesMu.Lock()
	respCachePointer := httpResponseCaches[url]
	if respCachePointer == nil {
		// Since this is the first time using this endpoint, prepare a Mutex for it.
		respCachePointer = &httpResponseCache{httpResponseMu: &sync.Mutex{}}
		httpResponseCaches[url] = respCachePointer
	}
	httpResponseCachesMu.Unlock()

	// So lengthy queries don't hold up short queries, use query-specific mutexes.
	respCachePointer.httpResponseMu.Lock()
	defer respCachePointer.httpResponseMu.Unlock()

	// When the same endpoint has been checked within the span of a single block, return the cached response.
	if time.Since(respCachePointer.httpResponseChecked) < constants.ThornadoBlockTime && respCachePointer.httpResponse != nil {
		return respCachePointer.httpResponse, http.StatusOK, nil
	}

	resp, err := b.httpClient.Get(url)
	if err != nil {
		b.errCounter.WithLabelValues("fail_get_from_thornado", "").Inc()
		return nil, http.StatusNotFound, fmt.Errorf("failed to GET from thornado: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			b.logger.Error().Err(closeErr).Msg("failed to close response body")
		}
	}()

	var buf []byte
	buf, err = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return buf, resp.StatusCode, errors.New("Status code: " + resp.Status + " returned")
	}
	if err != nil {
		b.errCounter.WithLabelValues("fail_read_thornado_resp", "").Inc()
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	// All being well with the response, save it to the cache.
	respCachePointer.httpResponse = buf
	respCachePointer.httpResponseChecked = time.Now()

	return buf, resp.StatusCode, nil
}

// getThorChainURL with the given path
func (b *thornadoBridge) getThorChainURL(path string) string {
	if strings.HasPrefix(b.cfg.ChainHost, "http") {
		return fmt.Sprintf("%s/%s", b.cfg.ChainHost, path)
	}

	uri := url.URL{
		Scheme: "http",
		Host:   b.cfg.ChainHost,
		Path:   path,
	}
	return uri.String()
}

// getAccountNumberAndSequenceNumber returns account and Sequence number required to post into thornado
func (b *thornadoBridge) getAccountNumberAndSequenceNumber() (uint64, uint64, error) {
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get signer address: %w", err)
	}
	path := fmt.Sprintf("%s/%s", AuthAccountEndpoint, signerAddr)

	body, _, err := b.getWithPath(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get auth accounts: %w", err)
	}

	var resp types.AccountResp
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, 0, fmt.Errorf("failed to unmarshal account resp: %w", err)
	}
	acc := resp.Account

	return acc.AccountNumber, acc.Sequence, nil
}

// GetConfig return the configuration
func (b *thornadoBridge) GetConfig() config.BifrostClientConfiguration {
	return b.cfg
}

// PostKeysignFailure generates and posts a keysign failure tx to Thornado.
func (b *thornadoBridge) PostKeysignFailure(blame stypes.Blame, height int64, coins common.Coins, pubkey common.PubKey) (common.TxID, error) {
	start := time.Now()
	defer func() {
		b.m.GetHistograms(metrics.SignToThornadoDuration).Observe(time.Since(start).Seconds())
	}()

	if blame.IsEmpty() {
		// MsgFrostKeysignFail will fail validation if having no FailReason.
		blame.FailReason = "no fail reason available"
	}
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		return common.BlankTxID, fmt.Errorf("failed to get signer address: %w", err)
	}
	msg, err := stypes.NewMsgFrostKeysignFail(height, blame, coins, signerAddr, pubkey)
	if err != nil {
		return common.BlankTxID, fmt.Errorf("fail to create keysign fail message: %w", err)
	}
	return b.Broadcast(msg)
}

// GetErrataMsg get errata tx from params
func (b *thornadoBridge) GetErrataMsg(txID common.TxID, chain common.Chain) sdk.Msg {
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		panic(err)
	}
	return stypes.NewMsgErrataTx(txID, chain, signerAddr)
}

// GetSolvencyMsg create MsgSolvency from the given parameters
func (b *thornadoBridge) GetSolvencyMsg(height int64, chain common.Chain, pubKey common.PubKey, coins common.Coins) *stypes.MsgSolvency {
	// To prevent different MsgSolvency ID incompatibility between nodes with different coin-observation histories,
	// only report coins for which the amounts are not currently 0.
	coins = coins.NoneEmpty()
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		panic(err)
	}
	msg, err := stypes.NewMsgSolvency(chain, pubKey, coins, height, signerAddr)
	if err != nil {
		b.logger.Err(err).Msg("fail to create MsgSolvency")
		return nil
	}
	return msg
}

// GetKeygenStdTx get keygen tx from params
func (b *thornadoBridge) GetKeygenStdTx(vaultPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []stypes.Blame, inputPks common.PubKeys, keygenType stypes.KeygenType, chains common.Chains, height, keygenTime int64, vaultPubKeyEddsa common.PubKey, keysharesBackupEddsa []byte) (sdk.Msg, error) {
	return b.GetKeygenStdTxWithFrost(vaultPubKey, secp256k1Signature, keysharesBackup, blame, inputPks, keygenType, chains, height, keygenTime, vaultPubKeyEddsa, keysharesBackupEddsa, nil)
}

func (b *thornadoBridge) GetKeygenStdTxWithFrost(vaultPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []stypes.Blame, inputPks common.PubKeys, keygenType stypes.KeygenType, chains common.Chains, height, keygenTime int64, vaultPubKeyEddsa common.PubKey, keysharesBackupEddsa, keysharesBackupFrost []byte) (sdk.Msg, error) {
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get signer address: %w", err)
	}

	return stypes.NewMsgKeygenVaultV2(inputPks.Strings(), vaultPubKey, secp256k1Signature, keysharesBackup, keygenType, height, blame, chains.Strings(), signerAddr, keygenTime, vaultPubKeyEddsa, keysharesBackupEddsa, keysharesBackupFrost)
}

// GetInboundOutbound separate the txs into inbound and outbound
func (b *thornadoBridge) GetInboundOutbound(txIns common.ObservedTxs) (common.ObservedTxs, common.ObservedTxs, error) {
	if len(txIns) == 0 {
		return nil, nil, nil
	}
	inbound := common.ObservedTxs{}
	outbound := common.ObservedTxs{}

	// spilt our txs into inbound vs outbound txs
	for _, tx := range txIns {
		var chain common.Chain
		if len(tx.Tx.Coins) > 0 {
			chain = tx.Tx.Coins[0].Asset.Chain
		}

		obAddr, err := tx.ObservedPubKey.GetAddress(chain)
		if err != nil {
			b.logger.Err(err).Msgf("fail to parse observed vault address: %s", tx.ObservedPubKey.String())
			continue
		}
		vaultToAddress := tx.Tx.ToAddress.Equals(obAddr)
		vaultFromAddress := tx.Tx.FromAddress.Equals(obAddr)
		if chain.Equals(common.BTCChain) {
			if !vaultToAddress {
				vaultToAddress = isDerivedBTCVaultAddress(tx.Tx.ToAddress, tx.ObservedPubKey)
			}
			if !vaultFromAddress {
				vaultFromAddress = isDerivedBTCVaultAddress(tx.Tx.FromAddress, tx.ObservedPubKey)
			}
		}
		var inInboundArray, inOutboundArray bool
		if vaultToAddress {
			inInboundArray = inbound.Contains(tx)
		}
		if vaultFromAddress {
			inOutboundArray = outbound.Contains(tx)
		}

		isCancelTransaction := tx.Tx.ToAddress.Equals(tx.Tx.FromAddress)

		// for consolidate UTXO tx, both From & To address will be the base address
		// thus here we need to make sure that one add to inbound , the other add to outbound
		switch {
		case vaultToAddress && vaultFromAddress && !tx.Tx.FromAddress.Equals(tx.Tx.ToAddress):
			if !inOutboundArray {
				outbound = append(outbound, tx)
			}
		case !vaultToAddress && !vaultFromAddress:
			// Neither ToAddress nor FromAddress matches obAddr, so drop it.
			b.logger.Error().Msgf("chain (%s) tx (%s) observedaddress (%s) does not match its toaddress (%s) or fromaddress (%s)", tx.Tx.Chain, tx.Tx.ID, obAddr, tx.Tx.ToAddress, tx.Tx.FromAddress)
		case vaultToAddress && !inInboundArray && !isCancelTransaction:
			inbound = append(inbound, tx)
		case vaultFromAddress && !inOutboundArray:
			outbound = append(outbound, tx)
		case inInboundArray && inOutboundArray:
			// It's already in both arrays, so drop it.
			b.logger.Error().Msgf("vault-to-vault chain (%s) tx (%s) is already in both inbound and outbound arrays", tx.Tx.Chain, tx.Tx.ID)
		case !vaultFromAddress && inInboundArray:
			// It's already in its only (inbound) array, so drop it.
			b.logger.Error().Msgf("observed tx in for chain (%s) tx (%s) is already in the inbound array", tx.Tx.Chain, tx.Tx.ID)
		case !vaultToAddress && inOutboundArray:
			// It's already in its only (outbound) array, so drop it.
			b.logger.Error().Msgf("observed tx out for chain (%s) tx (%s) is already in the outbound array", tx.Tx.Chain, tx.Tx.ID)
		default:
			// This should never happen; rather than dropping it, return an error.
			return nil, nil, fmt.Errorf("could not determine if chain (%s) tx (%s) was inbound or outbound", tx.Tx.Chain, tx.Tx.ID)
		}
	}

	return inbound, outbound, nil
}

func isDerivedBTCVaultAddress(addr common.Address, pubkey common.PubKey) bool {
	for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
		pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
		if err != nil {
			return false
		}
		for _, pathIndex := range pathIndexes {
			derived, err := common.DeriveBTCTaprootAddress(pubkey, pathIndex)
			if err != nil {
				return false
			}
			if addr.Equals(derived) {
				return true
			}
		}
	}
	return false
}

// EnsureNodeWhitelistedWithTimeout check node is whitelisted with timeout retry
func (b *thornadoBridge) EnsureNodeWhitelistedWithTimeout() error {
	for {
		select {
		case <-time.After(time.Hour):
			return errors.New("Observer is not whitelisted yet")
		default:
			err := b.EnsureNodeWhitelisted()
			if err == nil {
				// node had been whitelisted
				return nil
			}
			b.logger.Error().Err(err).Msg("observer is not whitelisted , will retry a bit later")
			time.Sleep(time.Second * 5)
		}
	}
}

// EnsureNodeWhitelisted will call to thornado to check whether the observer had been whitelist or not
func (b *thornadoBridge) EnsureNodeWhitelisted() error {
	status, err := b.FetchNodeStatus()
	if err != nil {
		return fmt.Errorf("failed to get node status: %w", err)
	}
	if status == stypes.NodeStatus_Unknown {
		return fmt.Errorf("node account status %s , will not be able to forward transaction to thornado", status)
	}
	return nil
}

func (b *thornadoBridge) FetchActiveNodes() ([]common.PubKey, error) {
	na, err := b.GetNodeAccounts()
	if err != nil {
		return nil, fmt.Errorf("fail to get node accounts: %w", err)
	}
	active := make([]common.PubKey, 0)
	for _, item := range na {
		// QueryNodeResponse.Status is a string, compare against the string representation
		if item.Status == stypes.NodeStatus_Active.String() {
			active = append(active, item.PubKeySet.Secp256k1)
		}
	}
	return active, nil
}

// FetchNodeStatus get current node status from thornado
func (b *thornadoBridge) FetchNodeStatus() (stypes.NodeStatus, error) {
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		return stypes.NodeStatus_Unknown, fmt.Errorf("fail to get signer address: %w", err)
	}
	bepAddr := signerAddr.String()
	if len(bepAddr) == 0 {
		return stypes.NodeStatus_Unknown, errors.New("bep address is empty")
	}
	na, err := b.GetNodeAccount(bepAddr)
	if err != nil {
		return stypes.NodeStatus_Unknown, fmt.Errorf("failed to get node status: %w", err)
	}
	return na.Status, nil
}

// GetKeysignParty call into thornado to get the node accounts that should be join together to sign the message
func (b *thornadoBridge) GetKeysignParty(vaultPubKey common.PubKey) (common.PubKeys, error) {
	p := fmt.Sprintf(SignerMembershipEndpoint, vaultPubKey.String())
	result, _, err := b.getWithPath(p)
	if err != nil {
		return common.PubKeys{}, fmt.Errorf("fail to get key sign party from thornado: %w", err)
	}
	var keys common.PubKeys
	if err = json.Unmarshal(result, &keys); err != nil {
		return common.PubKeys{}, fmt.Errorf("fail to unmarshal result to pubkeys:%w", err)
	}
	return keys, nil
}

// IsCatchingUp returns bool for if thornado is catching up to the rest of the
// nodes. Returns yes, if it is, false if it is caught up.
func (b *thornadoBridge) IsCatchingUp() (bool, error) {
	uri := url.URL{
		Scheme: "http",
		Host:   b.cfg.ChainRPC,
		Path:   StatusEndpoint,
	}

	body, _, err := b.get(uri.String())
	if err != nil {
		return false, fmt.Errorf("failed to get status data: %w", err)
	}

	var resp struct {
		Result struct {
			SyncInfo struct {
				CatchingUp bool `json:"catching_up"`
			} `json:"sync_info"`
		} `json:"result"`
	}

	if err = json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("failed to unmarshal tendermint status: %w", err)
	}
	return resp.Result.SyncInfo.CatchingUp, nil
}

// HasNetworkFee checks whether the BTC network fee has been posted.
func (b *thornadoBridge) HasNetworkFee(chain common.Chain) (bool, error) {
	transactionSize, _, err := b.GetNetworkFee(chain)
	if err != nil {
		return false, err
	}
	return transactionSize > 0, nil
}

// GetNetworkFee get chain's network fee from Thornado.
func (b *thornadoBridge) GetNetworkFee(chain common.Chain) (transactionSize, transactionFeeRate uint64, err error) {
	if !chain.Equals(common.BTCChain) {
		return 0, 0, fmt.Errorf("unsupported network fee chain: %s", chain)
	}
	buf, s, err := b.getWithPath(NetworkFeeEndpoint)
	if err != nil {
		return 0, 0, fmt.Errorf("fail to get network fee: %w", err)
	}
	if s != http.StatusOK {
		return 0, 0, fmt.Errorf("unexpected status code: %d", s)
	}
	var nf struct {
		Chain              common.Chain `json:"chain"`
		TransactionSize    any          `json:"transaction_size"`
		TransactionFeeRate any          `json:"transaction_fee_rate"`
	}
	if err = json.Unmarshal(buf, &nf); err != nil {
		return 0, 0, fmt.Errorf("fail to unmarshal network fee: %w", err)
	}
	transactionSize, err = parseNetworkFeeUint64(nf.TransactionSize)
	if err != nil {
		return 0, 0, fmt.Errorf("fail to unmarshal network fee transaction size: %w", err)
	}
	transactionFeeRate, err = parseNetworkFeeUint64(nf.TransactionFeeRate)
	if err != nil {
		return 0, 0, fmt.Errorf("fail to unmarshal network fee transaction fee rate: %w", err)
	}
	return transactionSize, transactionFeeRate, nil
}

func parseNetworkFeeUint64(v any) (uint64, error) {
	switch t := v.(type) {
	case string:
		return strconv.ParseUint(t, 10, 64)
	case float64:
		if t < 0 || t != float64(uint64(t)) {
			return 0, fmt.Errorf("invalid uint64 value: %v", t)
		}
		return uint64(t), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

// WaitToCatchUp wait for thornado to catch up
func (b *thornadoBridge) WaitToCatchUp() error {
	for {
		yes, err := b.IsCatchingUp()
		if err != nil {
			return err
		}
		if !yes {
			break
		}
		b.logger.Info().Msg("thornado is not caught up... waiting...")
		time.Sleep(constants.ThornadoBlockTime)
	}
	return nil
}

// GetBaseVaults retrieves the active base vaults from Thornado.
func (b *thornadoBridge) GetBaseVaults() (stypes.Vaults, error) {
	buf, s, err := b.getWithPath(BaseVaultEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fail to get base vaults: %w", err)
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", s)
	}
	var vaults stypes.Vaults
	if err = json.Unmarshal(buf, &vaults); err != nil {
		return nil, fmt.Errorf("fail to unmarshal base vaults from json: %w", err)
	}
	return vaults, nil
}

// GetVault retrieves a specific vault from thornado.
func (b *thornadoBridge) GetVault(pubkey string) (stypes.Vault, error) {
	buf, s, err := b.getWithPath(fmt.Sprintf(VaultEndpoint, pubkey))
	if err != nil {
		return stypes.Vault{}, fmt.Errorf("fail to get vault: %w", err)
	}
	if s != http.StatusOK {
		return stypes.Vault{}, fmt.Errorf("unexpected status code %d", s)
	}
	var vault stypes.Vault
	if err = json.Unmarshal(buf, &vault); err != nil {
		return stypes.Vault{}, fmt.Errorf("fail to unmarshal vault from json: %w", err)
	}
	return vault, nil
}

func (b *thornadoBridge) getVaultPubkeys() ([]byte, error) {
	buf, s, err := b.getWithPath(PubKeysEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fail to get vault pubkeys: %w", err)
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", s)
	}
	return buf, nil
}

// GetPubKeys retrieves active and inactive vault pubkeys.
// Returns both active and inactive vaults
func (b *thornadoBridge) GetPubKeys() ([]PubKeyAddressPair, error) {
	buf, err := b.getVaultPubkeys()
	if err != nil {
		return nil, fmt.Errorf("fail to get vault pubkeys ,err: %w", err)
	}
	var result struct {
		Base     []openapi.VaultInfo `json:"base"`
		Inactive []openapi.VaultInfo `json:"inactive"`
	}
	if err = json.Unmarshal(buf, &result); err != nil {
		return nil, fmt.Errorf("fail to unmarshal pubkeys: %w", err)
	}
	var addressPairs []PubKeyAddressPair

	// anonymous function to process a vault and add its pubkeys to addressPairs
	processVault := func(v openapi.VaultInfo, inactive bool) {
		membership := []common.PubKey{}
		for _, item := range v.Membership {
			membership = append(membership, common.PubKey(item))
		}

		// process eddsa pubkey if present
		if v.PubKeyEddsa != nil && *v.PubKeyEddsa != "" {
			kp := PubKeyAddressPair{
				PubKey:     common.PubKey(*v.PubKeyEddsa),
				Algo:       common.SigningAlgoEd25519,
				Membership: membership,
				Inactive:   inactive,
			}
			addressPairs = append(addressPairs, kp)
		}

		// process secp256k1 pubkey
		kp := PubKeyAddressPair{
			PubKey:     common.PubKey(v.PubKey),
			Algo:       common.SigningAlgoSecp256k1,
			Membership: membership,
			Inactive:   inactive,
		}
		addressPairs = append(addressPairs, kp)
	}

	// process active vaults
	for _, v := range result.Base {
		processVault(v, false)
	}

	// process inactive vaults
	for _, v := range result.Inactive {
		processVault(v, true)
	}

	return addressPairs, nil
}

// GetBasePubKeys retrieves base vault pubkeys.
func (b *thornadoBridge) GetBasePubKeys() ([]PubKeyAddressPair, error) {
	buf, err := b.getVaultPubkeys()
	if err != nil {
		return nil, fmt.Errorf("fail to get vault pubkeys ,err: %w", err)
	}
	var result struct {
		Base     []openapi.VaultInfo `json:"base"`
		Inactive []openapi.VaultInfo `json:"inactive"`
	}
	if err = json.Unmarshal(buf, &result); err != nil {
		return nil, fmt.Errorf("fail to unmarshal pubkeys: %w", err)
	}
	var addressPairs []PubKeyAddressPair
	for _, v := range append(result.Base, result.Inactive...) {
		if v.PubKeyEddsa != nil {
			kp := PubKeyAddressPair{
				PubKey: common.PubKey(*v.PubKeyEddsa),
				Algo:   common.SigningAlgoEd25519,
			}
			addressPairs = append(addressPairs, kp)
		}
		kp := PubKeyAddressPair{
			PubKey: common.PubKey(v.PubKey),
			Algo:   common.SigningAlgoSecp256k1,
		}
		addressPairs = append(addressPairs, kp)
	}
	return addressPairs, nil
}

// PostNetworkFee send network fee message to Thornado
func (b *thornadoBridge) PostNetworkFee(height int64, chain common.Chain, transactionSize, transactionRate uint64) (common.TxID, error) {
	nodeStatus, err := b.FetchNodeStatus()
	if err != nil {
		return common.BlankTxID, fmt.Errorf("failed to get node status: %w", err)
	}

	if nodeStatus != stypes.NodeStatus_Active {
		return common.BlankTxID, nil
	}
	start := time.Now()
	defer func() {
		b.m.GetHistograms(metrics.SignToThornadoDuration).Observe(time.Since(start).Seconds())
	}()
	signerAddr, err := b.keys.GetSignerInfo().GetAddress()
	if err != nil {
		return common.BlankTxID, fmt.Errorf("fail to get signer address: %w", err)
	}
	msg := stypes.NewMsgNetworkFee(height, chain, transactionSize, transactionRate, signerAddr)
	return b.Broadcast(msg)
}

// GetConstants returns grouped genesis defaults flattened for legacy callers.
func (b *thornadoBridge) GetConstants() (map[string]int64, error) {
	buf, s, err := b.getWithPath(ConfigDefaultsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fail to get config defaults: %w", err)
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", s)
	}
	return decodeConfigInt64Values(buf)
}

// GetThornadoVersion retrieve thornado version
func (b *thornadoBridge) GetThornadoVersion() (semver.Version, error) {
	buf, s, err := b.getWithPath(ChainVersionEndpoint)
	if err != nil {
		return semver.Version{}, fmt.Errorf("fail to get Thornado version: %w", err)
	}
	if s != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unexpected status code: %d", s)
	}
	var version openapi.VersionResponse
	if err = json.Unmarshal(buf, &version); err != nil {
		return semver.Version{}, fmt.Errorf("fail to unmarshal Thornado version : %w", err)
	}
	return semver.MustParse(version.Current), nil
}

// GetConfigValue returns an override value, or the genesis default when unset.
func (b *thornadoBridge) GetConfigValue(key string) (int64, error) {
	values, err := b.getConfigValues(ConfigEndpoint)
	if err != nil {
		return 0, err
	}
	if value, ok := lookupConfigValue(values, key); ok {
		return value, nil
	}
	values, err = b.getConfigValues(ConfigDefaultsEndpoint)
	if err != nil {
		return 0, err
	}
	if value, ok := lookupConfigValue(values, key); ok {
		return value, nil
	}
	return 0, fmt.Errorf("config key not found: %s", key)
}

// GetConfigWithRef inserts a reference into a Config key template.
func (b *thornadoBridge) GetConfigValueWithRef(template, ref string) (int64, error) {
	key := fmt.Sprintf(template, ref)
	return b.GetConfigValue(key)
}

func (b *thornadoBridge) getConfigValues(endpoint string) (map[string]int64, error) {
	buf, s, err := b.getWithPath(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fail to get config values: %w", err)
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", s)
	}
	return decodeConfigInt64Values(buf)
}

func decodeConfigInt64Values(buf []byte) (map[string]int64, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(buf, &top); err != nil {
		return nil, fmt.Errorf("fail to unmarshal config: %w", err)
	}
	values := make(map[string]int64)
	for key, raw := range top {
		if value, ok := parseConfigInt64(raw); ok {
			values[key] = value
			continue
		}

		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			continue
		}
		for name, raw := range entries {
			if value, ok := parseConfigInt64(raw); ok {
				values[key+"_"+name] = value
			}
		}
	}
	return values, nil
}

func parseConfigInt64(raw json.RawMessage) (int64, bool) {
	var entry struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &entry); err == nil && len(entry.Value) > 0 {
		raw = entry.Value
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return 0, false
	}
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func lookupConfigValue(values map[string]int64, key string) (int64, bool) {
	if value, ok := values[key]; ok {
		return value, true
	}
	normalizedKey := normalizeConfigKey(key)
	for candidate, value := range values {
		normalizedCandidate := normalizeConfigKey(candidate)
		if normalizedCandidate == normalizedKey || strings.HasSuffix(normalizedCandidate, normalizedKey) {
			return value, true
		}
	}
	return 0, false
}

func normalizeConfigKey(key string) string {
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return strings.ToUpper(key)
}

// PubKeyAddressPair is a vault pubkey plus signing metadata.
type PubKeyAddressPair struct {
	PubKey     common.PubKey
	Algo       common.SigningAlgo
	Membership []common.PubKey
	Inactive   bool
}
