package observer

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestObserverRoundTrip(t *testing.T) {
	types2.SetupConfigForTest()
	var txs []*types.TxIn
	deckBz, err := os.ReadFile("../../test/fixtures/observer/deck.json")
	require.NoError(t, err)
	err = json.Unmarshal(deckBz, &txs)
	require.NoError(t, err)

	server := httptest.NewServer(
		http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			switch {
			case strings.HasPrefix(req.RequestURI, thorclient.MimirEndpoint):
				buf, readErr := os.ReadFile("../../test/fixtures/endpoints/mimir/mimir.json")
				require.NoError(t, readErr)
				_, writeErr := rw.Write(buf)
				require.NoError(t, writeErr)
			case strings.HasPrefix(req.RequestURI, "/thorchain/lastblock"):
				// NOTE: weird pattern in GetBlockHeight uses first thorchain height.
				_, writeErr := rw.Write([]byte(`[
          {
            "chain": "NOOP",
            "lastobservedin": 0,
            "lastsignedout": 0,
            "thorchain": 0
          }
        ]`))
				require.NoError(t, writeErr)
			case strings.HasPrefix(req.RequestURI, "/"):
				_, writeErr := rw.Write([]byte(`{
          "jsonrpc": "2.0",
          "id": 0,
          "result": {
            "height": "1",
            "hash": "E7FDA9DE4D0AD37D823813CB5BC0D6E69AB0D41BB666B65B965D12D24A3AE83C",
            "logs": [
              {
                "success": "true",
                "log": ""
              }
            ]
          }
        }`))
				require.NoError(t, writeErr)
			default:
				t.Fatalf("invalid server query: %s", req.RequestURI)
			}
		}))

	cfg := config.BifrostClientConfiguration{
		ChainID:      "thorchain",
		ChainHost:    server.Listener.Addr().String(),
		ChainRPC:     server.Listener.Addr().String(),
		SignerName:   "bob",
		SignerPasswd: "password",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err = kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	require.NoError(t, err)
	thorKeys := thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)

	require.NotNil(t, thorKeys)
	bridge, err := thorclient.NewThorchainBridge(cfg, nil, thorKeys)
	require.NotNil(t, bridge)
	require.NoError(t, err)
	priv, err := thorKeys.GetPrivateKey()
	require.NoError(t, err)
	tmp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	require.NoError(t, err)
	_, err = common.NewPubKeyFromCrypto(tmp)
	require.NoError(t, err)

	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(bridge, nil)
	require.NoError(t, err)
	comm, err := p2p.NewCommunication(&p2p.Config{
		RendezvousString: "rendezvous",
		Port:             1234,
	}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, comm)
	err = comm.Start(priv.Bytes())
	require.NoError(t, err)

	defer func() {
		stopErr := comm.Stop()
		require.NoError(t, stopErr)
	}()

	require.NotNil(t, comm.GetHost())

	ag, err := NewAttestationGossip(comm.GetHost(), thorKeys, "localhost:50051", bridge, nil, config.BifrostAttestationGossipConfig{})
	require.NoError(t, err)

	tmpDir := t.TempDir()

	obs, err := NewObserver(pubkeyMgr, nil, bridge, nil, tmpDir, metrics.NewTssKeysignMetricMgr(), ag, "")
	require.NoError(t, err)
	require.NotNil(t, obs)
	ag.SetObserverHandleObservedTxCommitted(obs)

	obs.chains = make(map[common.Chain]chainclients.ChainClient)
	obs.chains[common.BCHChain] = &mockChainClient{}
	obs.chains[common.ETHChain] = &mockChainClient{}
	obs.chains[common.LTCChain] = &mockChainClient{}

	for _, tx := range txs {
		err = obs.storage.AddOrUpdateTx(tx)
		require.NoError(t, err)

		obs.onDeck[TxInKey(tx)] = tx

		for _, txi := range tx.TxArray {
			pubkeyMgr.AddPubKey(txi.ObservedVaultPubKey, false, common.SigningAlgoSecp256k1)
		}
	}

	require.Len(t, obs.onDeck, len(txs))
	dbTxs, err := obs.storage.GetOnDeckTxs()
	require.NoError(t, err)
	require.Len(t, dbTxs, len(txs))

	for _, tx := range dbTxs {
		final := false

		obsTxs, invalidIndices, txErr := obs.getThorchainTxIns(tx, final, tx.TxArray[0].BlockHeight+tx.ConfirmationRequired)
		require.NoError(t, txErr)
		require.Empty(t, invalidIndices, "expected no invalid transactions in test data")

		inbound, outbound, getErr := bridge.GetInboundOutbound(obsTxs)
		require.NoError(t, getErr)

		rand.Shuffle(len(inbound), func(i, j int) {
			inbound[i], inbound[j] = inbound[j], inbound[i]
		})

		rand.Shuffle(len(outbound), func(i, j int) {
			outbound[i], outbound[j] = outbound[j], outbound[i]
		})

		for _, inb := range inbound {
			obs.handleObservedTxCommitted(inb)
		}
		for _, outb := range outbound {
			obs.handleObservedTxCommitted(outb)
		}
	}

	for _, tx := range dbTxs {
		numTxs := len(tx.TxArray)

		final := true

		obsTxs, invalidIndices, txErr := obs.getThorchainTxIns(tx, final, tx.TxArray[0].BlockHeight+tx.ConfirmationRequired)
		require.NoError(t, txErr)
		require.Empty(t, invalidIndices, "expected no invalid transactions in test data")

		require.Len(t, obsTxs, numTxs)

		inbound, outbound, getErr := bridge.GetInboundOutbound(obsTxs)
		require.NoError(t, getErr)

		require.GreaterOrEqual(t, len(inbound)+len(outbound), numTxs)

		rand.Shuffle(len(inbound), func(i, j int) {
			inbound[i], inbound[j] = inbound[j], inbound[i]
		})

		rand.Shuffle(len(outbound), func(i, j int) {
			outbound[i], outbound[j] = outbound[j], outbound[i]
		})

		for _, inb := range inbound {
			obs.handleObservedTxCommitted(inb)
		}
		for _, outb := range outbound {
			obs.handleObservedTxCommitted(outb)
		}
	}

	require.Len(t, obs.onDeck, 0)
	dbTxs, err = obs.storage.GetOnDeckTxs()
	require.NoError(t, err)
	require.Len(t, dbTxs, 0)
}
