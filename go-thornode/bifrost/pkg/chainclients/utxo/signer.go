package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	bchwire "github.com/gcash/bchd/wire"
	"github.com/gcash/bchutil"
	"github.com/hashicorp/go-multierror"
	ltcwire "github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcutil"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/utxo/zecutil"

	ltctxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/ltcd-txscript"

	btcwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	dogewire "github.com/eager7/dogd/wire"
	"github.com/eager7/dogutil"

	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/utxo"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"

	"github.com/btcsuite/btcd/mempool"
)

// ZecTxExpiry (in blocks) is used to calculate the tx expiry height
const ZecTxExpiry = uint32(40)

// ZecExtraFee (in Zats) is added to the calculated gas costs
const ZecExtraFee = int(0)

////////////////////////////////////////////////////////////////////////////////////////
// Client - Signing
////////////////////////////////////////////////////////////////////////////////////////

// SignTx builds and signs the outbound transaction. Returns the signed transaction, a
// serialized checkpoint on error, and an error.
func (c *Client) SignTx(tx stypes.TxOutItem, thorchainHeight int64) ([]byte, []byte, *stypes.TxInItem, error) {
	if !tx.Chain.Equals(c.cfg.ChainID) {
		return nil, nil, nil, errors.New("wrong chain")
	}

	// skip outbounds without coins
	if tx.Coins.IsEmpty() {
		return nil, nil, nil, nil
	}

	if c.cfg.ChainID.Equals(common.BCHChain) {
		if !tx.ToAddress.IsValidBCHAddress() {
			c.log.Error().Msgf("to address: %s is legacy not allowed ", tx.ToAddress)
			return nil, nil, nil, nil
		}
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

	// get chain specific address type
	var outputAddr interface{}
	var outputAddrStr string
	switch c.cfg.ChainID {
	case common.DOGEChain:
		outputAddr, err = dogutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgDOGE())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = outputAddr.(dogutil.Address).String() // trunk-ignore(golangci-lint/forcetypeassert)
	case common.BCHChain:
		outputAddr, err = bchutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBCH())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = outputAddr.(bchutil.Address).String() // trunk-ignore(golangci-lint/forcetypeassert)
	case common.LTCChain:
		outputAddr, err = ltctxscript.DecodeAddress(tx.ToAddress.String(), c.getChainCfgLTC())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = outputAddr.(ltcutil.Address).String() // trunk-ignore(golangci-lint/forcetypeassert)
	case common.BTCChain:
		outputAddr, err = btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = outputAddr.(btcutil.Address).String()
	case common.ZECChain:
		outputAddr, err = zecutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgZEC().Name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		// re-encoding addresses would break tex address support
		outputAddrStr = tx.ToAddress.String()
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	// verify address
	if !strings.EqualFold(outputAddrStr, tx.ToAddress.String()) {
		c.log.Info().Msgf("output address: %s, to address: %s can't roundtrip", outputAddrStr, tx.ToAddress.String())
		return nil, nil, nil, nil
	}
	switch outputAddr.(type) {
	case *dogutil.AddressPubKey, *bchutil.AddressPubKey, *ltcutil.AddressPubKey, *btcutil.AddressPubKey:
		c.log.Info().Msgf("address: %s is address pubkey type, should not be used", outputAddrStr)
		return nil, nil, nil, nil
	default: // keep lint happy
	}

	// load from checkpoint if it exists
	checkpoint := utxo.SignCheckpoint{}
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

	// convert the wire tx to the chain specific tx for signing
	var stx interface{}
	switch c.cfg.ChainID {
	case common.DOGEChain:
		stx = wireToDOGE(redeemTx)
	case common.BCHChain:
		stx = wireToBCH(redeemTx)
	case common.LTCChain:
		stx = wireToLTC(redeemTx)
	case common.BTCChain:
		stx = wireToBTC(redeemTx)
	case common.ZECChain:
		// signing is slightly different, using zecTx, no need to wire
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	chainHeight, err := c.rpc.GetBlockCount()
	if err != nil {
		// fall back to the scanner height, thornode voter does not use height
		chainHeight = c.getCurrentBlockHeight()
		c.log.Warn().Err(err).
			Int64("fallback_height", chainHeight).
			Msg("failed to get block height from RPC, falling back to scanner height")
	}

	// only used for zcash
	var zecTx *zecutil.MsgTx
	if c.cfg.ChainID == common.ZECChain {
		// WARNING: v4 (Sapling) is assumed by both BroadcastTx (double-SHA256 txid)
		// and zecutil signing (Blake2b sighash). Upgrading to v5 requires updating
		// txid calculation per ZIP-244 and the signing algorithm.
		redeemTx.Version = 4

		zecTx = &zecutil.MsgTx{
			MsgTx:        redeemTx,
			ExpiryHeight: uint32(chainHeight) + ZecTxExpiry,
		}
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

			// chain specific signing
			switch c.cfg.ChainID {
			case common.DOGEChain:
				err = c.signUTXODOGE(stx.(*dogewire.MsgTx), tx, amount, sourceScript, i)
			case common.BCHChain:
				err = c.signUTXOBCH(stx.(*bchwire.MsgTx), tx, amount, sourceScript, i)
			case common.LTCChain:
				err = c.signUTXOLTC(stx.(*ltcwire.MsgTx), tx, amount, sourceScript, i)
			case common.BTCChain:
				err = c.signUTXOBTC(stx.(*btcwire.MsgTx), tx, amount, sourceScript, i)
			case common.ZECChain:
				err = c.signUTXOZEC(zecTx, tx, amount, sourceScript, i)
			default:
				c.log.Fatal().Msg("unsupported chain")
			}

			if err != nil {
				mu.Lock()
				utxoErr = multierror.Append(utxoErr, err)
				mu.Unlock()
			}
		}(int(signing.idx), signing.amount)
	}
	wg.Wait()
	if utxoErr != nil {
		err = utxo.PostKeysignFailure(c.bridge, tx, c.log, thorchainHeight, utxoErr)
		return nil, checkpointBytes, nil, fmt.Errorf("fail to sign the message: %w", err)
	}

	// convert back to wire tx
	switch c.cfg.ChainID {
	case common.DOGEChain:
		redeemTx = dogeToWire(stx.(*dogewire.MsgTx))
	case common.BCHChain:
		redeemTx = bchToWire(stx.(*bchwire.MsgTx))
	case common.LTCChain:
		redeemTx = ltcToWire(stx.(*ltcwire.MsgTx))
	case common.BTCChain:
		redeemTx = btcToWire(stx.(*btcwire.MsgTx))
	case common.ZECChain:
		// using zecTx directly, no need to convert back
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	var signedTx bytes.Buffer

	switch c.cfg.ChainID {
	case common.ZECChain:
		err = zecTx.ZecEncode(&signedTx, 0, btcwire.BaseEncoding)
	default:
		// calculate the final transaction size
		finalSize := redeemTx.SerializeSize()
		finalVBytes := mempool.GetTxVirtualSize(btcutil.NewTx(redeemTx))
		c.log.Info().Msgf("final size: %d, final vbyte: %d", finalSize, finalVBytes)

		err = redeemTx.Serialize(&signedTx)
	}

	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to serialize tx to bytes: %w", err)
	}

	// create the observation to be sent by the signer before broadcast
	amt := redeemTx.TxOut[0].Value // the first output is the outbound amount
	gas := totalAmount
	for _, txOut := range redeemTx.TxOut { // subtract all vouts to from vins to get the gas
		gas -= txOut.Value
	}

	// ZEC hash is computed from zecTx instead of redeemTx
	txHash := redeemTx.TxHash().String()
	if c.cfg.ChainID == common.ZECChain {
		txHash = zecTx.TxHash().String()
	}

	var txIn *stypes.TxInItem
	sender, err := tx.VaultPubKey.GetAddress(tx.Chain)
	if err == nil {
		txIn = stypes.NewTxInItem(
			chainHeight,
			txHash,
			tx.Memo,
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

	// keep track of used utxos for Zcash
	if c.cfg.ChainID == common.ZECChain {
		var ids []string
		for _, vin := range redeemTx.TxIn {
			id := formatUtxoKey(vin.PreviousOutPoint.Hash.String(), vin.PreviousOutPoint.Index)
			ids = append(ids, id)
		}
		err = c.temporalStorage.SetSpentUtxos(ids, chainHeight)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to set utxos: %w", err)
		}
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
		bm = utxo.NewBlockMeta("", height, "")
	}
	defer func() {
		// trunk-ignore(golangci-lint/govet): shadow
		if err := c.temporalStorage.SaveBlockMeta(height, bm); err != nil {
			c.log.Err(err).Msg("fail to save block metadata")
		}
	}()

	redeemTx := btcwire.NewMsgTx(btcwire.TxVersion)

	// broadcast tx
	var txid string
	switch c.cfg.ChainID {
	case common.ZECChain:
		if len(payload) == 0 {
			return "", fmt.Errorf("payload is empty")
		}

		args := []any{hex.EncodeToString(payload)}

		err = c.rpc.Call(&txid, "sendrawtransaction", args...)
		if txid == "" {
			c.log.Error().Msg("fail to call SendRawTransaction")
		}
	default:
		buf := bytes.NewBuffer(payload)
		if err = redeemTx.Deserialize(buf); err != nil {
			return "", fmt.Errorf("fail to deserialize payload: %w", err)
		}

		var maxFee any
		switch c.cfg.ChainID {
		case common.DOGEChain, common.BCHChain:
			maxFee = true // "allowHighFees"
		case common.LTCChain, common.BTCChain:
			maxFee = 10_000_000
		}

		txid, err = c.rpc.SendRawTransaction(redeemTx, maxFee)
	}

	if txid != "" {
		bm.AddSelfTransaction(txid)
	}

	msgs := []string{"already in block chain"}

	if err != nil {
		switch c.cfg.ChainID {
		case common.ZECChain:
			hash1 := sha256.Sum256(payload)
			hash2 := sha256.Sum256(hash1[:])
			final := hash2[:]

			slices.Reverse(final)

			txid = hex.EncodeToString(final)

			// zebrad specific messages
			msgs = append(msgs, []string{
				"already exists in mempool",
				"transaction was committed to the best chain",
			}...)
		default:
			txid = redeemTx.TxHash().String()
		}

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
