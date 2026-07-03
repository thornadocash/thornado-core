package observer

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/thornadocash/go-thornado/bifrost/pubkeymanager"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/ebifrost"
)

type ManualObserveResult struct {
	TxID                      string
	Chain                     common.Chain
	Mempool                   bool
	Finalized                 bool
	Inbound                   int
	Outbound                  int
	AllowFutureObservation    bool
	RequiredConfirmations     int64
	ObservationFinaliseHeight int64
}

// ManualObserveTxIDs is a one-shot operator recovery path. It observes through the
// local chain client and submits only this node's attestation to Thornado eBifrost.
func ManualObserveTxIDs(
	ctx context.Context,
	keys *thornadoclient.Keys,
	thornadoBifrostGRPCAddress string,
	bridge thornadoclient.ThornadoBridge,
	pubkeyMgr pubkeymanager.PubKeyValidator,
	chainClient *btc.Client,
	txids []string,
	allowFutureObservation bool,
) ([]ManualObserveResult, error) {
	if len(txids) == 0 {
		return nil, fmt.Errorf("at least one txid is required")
	}
	cc, err := grpc.NewClient(thornadoBifrostGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer cc.Close()

	privKey, err := keys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get private key: %w", err)
	}
	pubKey := privKey.PubKey().Bytes()
	client := ebifrost.NewLocalhostBifrostClient(cc)

	results := make([]ManualObserveResult, 0, len(txids))
	for _, txid := range txids {
		txid = strings.TrimSpace(txid)
		if txid == "" {
			continue
		}
		txIn, err := chainClient.ObserveTxIn(txid)
		if err != nil {
			return results, fmt.Errorf("observe %s: %w", txid, err)
		}
		txIn.AllowFutureObservation = allowFutureObservation
		txIn.TxArray = filterManualObservations(pubkeyMgr, txIn.Chain, txIn.TxArray)
		if len(txIn.TxArray) == 0 {
			return results, fmt.Errorf("observe %s: no Thornado vault match", txid)
		}
		txIn.ConfirmationRequired = chainClient.GetConfirmationCount(txIn)
		finalized := !txIn.MemPool && chainClient.ConfirmationCountReady(txIn)
		finaliseHeight := finaliseHeight(txIn.TxArray[0].BlockHeight, txIn.ConfirmationRequired)
		obsTxs, err := manualObservedTxsFromTxIn(&txIn, finalized, finaliseHeight)
		if err != nil {
			return results, fmt.Errorf("observe %s: %w", txid, err)
		}
		inbound, outbound, err := bridge.GetInboundOutbound(obsTxs)
		if err != nil {
			return results, fmt.Errorf("observe %s: classify inbound/outbound: %w", txid, err)
		}
		for _, tx := range inbound {
			if err := submitManualObservedTx(ctx, client, privKey, pubKey, tx, true, allowFutureObservation); err != nil {
				return results, fmt.Errorf("observe %s inbound: %w", txid, err)
			}
		}
		for _, tx := range outbound {
			if err := submitManualObservedTx(ctx, client, privKey, pubKey, tx, false, allowFutureObservation); err != nil {
				return results, fmt.Errorf("observe %s outbound: %w", txid, err)
			}
		}
		results = append(results, ManualObserveResult{
			TxID:                      txid,
			Chain:                     txIn.Chain,
			Mempool:                   txIn.MemPool,
			Finalized:                 finalized,
			Inbound:                   len(inbound),
			Outbound:                  len(outbound),
			AllowFutureObservation:    allowFutureObservation,
			RequiredConfirmations:     txIn.ConfirmationRequired,
			ObservationFinaliseHeight: finaliseHeight,
		})
	}
	return results, nil
}

func submitManualObservedTx(
	ctx context.Context,
	client ebifrost.LocalhostBifrostClient,
	privKey interface{ Sign([]byte) ([]byte, error) },
	pubKey []byte,
	obsTx common.ObservedTx,
	inbound, allowFutureObservation bool,
) error {
	signBz, err := obsTx.GetSignablePayloadWithInbound(inbound)
	if err != nil {
		return fmt.Errorf("fail to marshal tx sign payload: %w", err)
	}
	signature, err := privKey.Sign(signBz)
	if err != nil {
		return fmt.Errorf("fail to sign tx sign payload: %w", err)
	}
	_, err = client.SendQuorumTx(ctx, &common.QuorumTx{
		ObsTx:                  obsTx,
		Inbound:                inbound,
		AllowFutureObservation: allowFutureObservation,
		Attestations: []*common.Attestation{{
			PubKey:    pubKey,
			Signature: signature,
		}},
	})
	if err != nil {
		return fmt.Errorf("fail to send quorum tx: %w", err)
	}
	return nil
}

func filterManualObservations(pubkeyMgr pubkeymanager.PubKeyValidator, chain common.Chain, items []*types.TxInItem) []*types.TxInItem {
	var txs []*types.TxInItem
	for _, txInItem := range items {
		if txInItem == nil {
			continue
		}
		if ok, cpi := pubkeyMgr.IsValidVaultAddress(txInItem.Sender, chain); ok {
			tx := txInItem.Copy()
			tx.ObservedVaultPubKey = cpi.PubKey
			txs = append(txs, tx)
		}
		if txInItem.Sender == txInItem.To {
			continue
		}
		if ok, cpi := pubkeyMgr.IsValidVaultAddress(txInItem.To, chain); ok {
			tx := txInItem.Copy()
			tx.ObservedVaultPubKey = cpi.PubKey
			txs = append(txs, tx)
		}
	}
	return txs
}

func manualObservedTxsFromTxIn(txIn *types.TxIn, finalized bool, finaliseHeight int64) (common.ObservedTxs, error) {
	obsTxs := make(common.ObservedTxs, 0, len(txIn.TxArray))
	for _, item := range txIn.TxArray {
		if item == nil || item.CommittedUnFinalised && !finalized {
			continue
		}
		if item.Coins.IsEmpty() {
			return nil, fmt.Errorf("item %s has empty coins", item.Tx)
		}
		if len(item.To) == 0 {
			return nil, fmt.Errorf("tx %s has empty to address", item.Tx)
		}
		txID, err := common.NewTxID(item.Tx)
		if err != nil {
			return nil, fmt.Errorf("parse tx hash %s: %w", item.Tx, err)
		}
		sender, err := common.NewAddress(item.Sender)
		if err != nil {
			return nil, fmt.Errorf("parse sender %s: %w", item.Sender, err)
		}
		to, err := common.NewAddress(item.To)
		if err != nil {
			return nil, fmt.Errorf("parse to %s: %w", item.To, err)
		}
		if _, err := item.ObservedVaultPubKey.GetAddress(txIn.Chain); err != nil {
			return nil, fmt.Errorf("parse observed vault address %s: %w", item.ObservedVaultPubKey.String(), err)
		}
		height := item.BlockHeight
		if txIn.MemPool && !finalized && !txIn.AllowFutureObservation {
			height = 0
		}
		if finalized && height == 0 {
			height = finaliseHeight
		}
		tx := common.NewTx(txID, sender, to, item.Coins.NoneEmpty(), item.Gas.NoneEmpty())
		tx.SourceVout = item.SourceVout
		tx.SourceInputs = types.ToCommonTxInputs(item.SourceInputs)
		obsTxs = append(obsTxs, common.NewObservedTx(tx, height, item.ObservedVaultPubKey, finaliseHeight))
	}
	return obsTxs, nil
}
