package utxo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"

	btcwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"

	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"

	"github.com/btcsuite/btcd/mempool"
)

////////////////////////////////////////////////////////////////////////////////////////
// Client - Signing
////////////////////////////////////////////////////////////////////////////////////////

// SignTx builds and signs the outbound transaction. Returns the signed transaction, a
// serialized checkpoint on error, and an error.
func (c *Client) SignTx(tx stypes.TxOutItem, thornadoHeight int64) ([]byte, []byte, *stypes.TxInItem, error) {
	if !tx.Chain.Equals(c.cfg.ChainID) {
		return nil, nil, nil, errors.New("wrong chain")
	}

	// skip outbounds without coins
	if tx.Coins.IsEmpty() {
		return nil, nil, nil, nil
	}

	// skip outbounds that have been signed
	if c.signerCacheManager.HasSigned(tx.CacheHash()) {
		c.log.Info().Msgf("ignoring already signed transaction: (%+v)", tx)
		return nil, nil, nil, nil
	}

	sourceScript, err := c.getSourceScript(tx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get source pay to address script: %w", err)
	}

	outputAddr, err := btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
	}

	// verify address
	if !strings.EqualFold(outputAddr.String(), tx.ToAddress.String()) {
		c.log.Info().Msgf("output address: %s, to address: %s can't roundtrip", outputAddr.String(), tx.ToAddress.String())
		return nil, nil, nil, nil
	}
	switch outputAddr.(type) {
	case *btcutil.AddressPubKey:
		c.log.Info().Msgf("address: %s is address pubkey type, should not be used", outputAddr.String())
		return nil, nil, nil, nil
	default: // keep lint happy
	}

	// load from checkpoint if it exists
	checkpoint := btc.SignCheckpoint{}
	redeemTx := &btcwire.MsgTx{}
	if tx.Checkpoint != nil {
		if err = json.Unmarshal(tx.Checkpoint, &checkpoint); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to unmarshal checkpoint: %w", err)
		}
		if err = redeemTx.Deserialize(bytes.NewReader(checkpoint.UnsignedTx)); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to deserialize tx: %w", err)
		}

		// abort if any checkpoint VIN is spent
		c.log.Info().Stringer("in_hash", tx.InHash).Msgf("verifying checkpoint vins")
		var unspent bool
		unspent, err = c.vinsUnspent(tx, redeemTx.TxIn)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to verify checkpoint vins: %w", err)
		}
		if !unspent {
			return nil, nil, nil, nil
		}

	} else {
		redeemTx, checkpoint.IndividualAmounts, err = c.buildTx(tx, sourceScript)
		if err != nil {
			if tx.TxType == "sweep" &&
				thornadoHeight > tx.Height+10 &&
				strings.Contains(err.Error(), "insufficient available UTXOs") {
				c.log.Warn().
					Stringer("in_hash", tx.InHash).
					Uint64("vault_path_index", tx.VaultPathIndex).
					Int64("tx_height", tx.Height).
					Int64("thornado_height", thornadoHeight).
					Err(err).
					Msg("skipping stale BTC sweep with no spendable source UTXO")
				return nil, nil, nil, nil
			}
			return nil, nil, nil, err
		}
		buf := bytes.NewBuffer([]byte{})
		err = redeemTx.Serialize(buf)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to serialize tx: %w", err)
		}
		checkpoint.UnsignedTx = buf.Bytes()
	}

	// serialize the checkpoint for later
	var checkpointBytes []byte
	checkpointBytes, err = json.Marshal(checkpoint)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to marshal checkpoint: %w", err)
	}

	// create the list of signing requests
	c.log.Info().Msgf("UTXOs to sign: %d", len(redeemTx.TxIn))
	signings := []struct{ idx, amount int64 }{}
	totalAmount := int64(0)
	for idx, txIn := range redeemTx.TxIn {
		key := formatUtxoKey(txIn.PreviousOutPoint.Hash.String(), txIn.PreviousOutPoint.Index)
		outputAmount := checkpoint.IndividualAmounts[key]
		totalAmount += outputAmount
		signings = append(signings, struct{ idx, amount int64 }{int64(idx), outputAmount})
	}

	chainHeight, err := c.rpc.GetBlockCount()
	if err != nil {
		// fall back to the scanner height, thornado voter does not use height
		chainHeight = c.getCurrentBlockHeight()
		c.log.Warn().Err(err).
			Int64("fallback_height", chainHeight).
			Msg("failed to get block height from RPC, falling back to scanner height")
	}

	// sign the tx
	wg := &sync.WaitGroup{}
	wg.Add(len(signings))
	mu := &sync.Mutex{}
	var utxoErr error
	for _, signing := range signings {
		go func(i int, amount int64) {
			defer wg.Done()

			// trunk-ignore(golangci-lint/govet): shadow
			var err error

			err = c.signUTXOBTC(redeemTx, tx, amount, sourceScript, i)

			if err != nil {
				mu.Lock()
				utxoErr = multierror.Append(utxoErr, err)
				mu.Unlock()
			}
		}(int(signing.idx), signing.amount)
	}
	wg.Wait()
	if utxoErr != nil {
		err = btc.PostKeysignFailure(c.bridge, tx, c.log, thornadoHeight, utxoErr)
		return nil, checkpointBytes, nil, fmt.Errorf("fail to sign the message: %w", err)
	}

	var signedTx bytes.Buffer
	finalSize := redeemTx.SerializeSize()
	finalVBytes := mempool.GetTxVirtualSize(btcutil.NewTx(redeemTx))
	c.log.Info().Msgf("final size: %d, final vbyte: %d", finalSize, finalVBytes)
	err = redeemTx.Serialize(&signedTx)

	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to serialize tx to bytes: %w", err)
	}

	// create the observation to be sent by the signer before broadcast
	amt := redeemTx.TxOut[0].Value // the first output is the outbound amount
	gas := totalAmount
	for _, txOut := range redeemTx.TxOut { // subtract all vouts to from vins to get the gas
		gas -= txOut.Value
	}

	txHash := redeemTx.TxHash().String()

	var txIn *stypes.TxInItem
	sender, err := c.getVaultAddressAtPath(tx.VaultPubKey, tx.VaultPathIndex)
	if err == nil {
		txIn = stypes.NewTxInItem(
			chainHeight,
			txHash,
			sender.String(),
			tx.ToAddress.String(),
			common.NewCoins(
				common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(amt))),
			),
			common.Gas(common.NewCoins(
				common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(gas))),
			)),
			tx.VaultPubKey,
			"",
			"",
			nil,
		)
	}

	return signedTx.Bytes(), nil, txIn, nil
}

// GetVaultLock returns a mutex for the given vault pubkey. This is primarily used to
// ensure transactions from the signer do not conflict with consolidate transactions.
func (c *Client) GetVaultLock(vaultPubKey string) *sync.Mutex {
	c.signerLock.Lock()
	defer c.signerLock.Unlock()
	l, ok := c.vaultLocks[vaultPubKey]
	if !ok {
		newLock := &sync.Mutex{}
		c.vaultLocks[vaultPubKey] = newLock
		return newLock
	}
	return l
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Broadcast
////////////////////////////////////////////////////////////////////////////////////////

// BroadcastTx will broadcast the given payload.
func (c *Client) BroadcastTx(txOut stypes.TxOutItem, payload []byte) (string, error) {
	height, err := c.rpc.GetBlockCount()
	if err != nil {
		return "", fmt.Errorf("fail to get block height: %w", err)
	}
	bm, err := c.temporalStorage.GetBlockMeta(height)
	if err != nil {
		c.log.Err(err).Int64("height", height).Msg("fail to get blockmeta")
	}
	if bm == nil {
		bm = btc.NewBlockMeta("", height, "")
	}
	defer func() {
		// trunk-ignore(golangci-lint/govet): shadow
		if err := c.temporalStorage.SaveBlockMeta(height, bm); err != nil {
			c.log.Err(err).Msg("fail to save block metadata")
		}
	}()

	redeemTx := btcwire.NewMsgTx(btcwire.TxVersion)

	buf := bytes.NewBuffer(payload)
	if err = redeemTx.Deserialize(buf); err != nil {
		return "", fmt.Errorf("fail to deserialize payload: %w", err)
	}

	txid, err := c.rpc.SendRawTransaction(redeemTx, 0.10)

	if txid != "" {
		bm.AddSelfTransaction(txid)
	}

	msgs := []string{"already in block chain"}

	if err != nil {
		txid = redeemTx.TxHash().String()

		for _, msg := range msgs {
			if strings.Contains(err.Error(), msg) {
				c.log.Info().Str("hash", txid).Msg("broadcasted by another node")
				err = c.signerCacheManager.SetSigned(txOut.CacheHash(), txOut.CacheVault(c.GetChain()), txid)
				if err != nil {
					c.log.Err(err).Msgf("fail to mark tx out item (%+v) as signed", txOut)
				}
				return txid, nil
			}
		}

		return "", fmt.Errorf("fail to broadcast transaction to chain: %w", err)
	}

	// save tx id to block meta in case we need to errata later
	if err = c.signerCacheManager.SetSigned(txOut.CacheHash(), txOut.CacheVault(c.GetChain()), txid); err != nil {
		c.log.Err(err).Msgf("fail to mark tx out item (%+v) as signed", txOut)
	}

	return txid, nil
}
