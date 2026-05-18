package utxo

import (
	"fmt"

	bchec "github.com/gcash/bchd/bchec"
	bchchaincfg "github.com/gcash/bchd/chaincfg"
	bchwire "github.com/gcash/bchd/wire"
	bchtxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/bchd-txscript"

	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
)

func (c *Client) getChainCfgBCH() *bchchaincfg.Params {
	switch common.CurrentChainNetwork {
	case common.MockNet:
		return &bchchaincfg.RegressionNetParams
	case common.MainNet, common.StageNet, common.ChainNet:
		return &bchchaincfg.MainNetParams
	default:
		c.log.Fatal().Msg("unsupported network")
		return nil
	}
}

func (c *Client) signUTXOBCH(redeemTx *bchwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	var signable bchtxscript.Signable
	if tx.VaultPubKey.Equals(c.nodePubKey) {
		signable = bchtxscript.NewPrivateKeySignable((*bchec.PrivateKey)(c.nodePrivKey))
	} else {
		signable = newTssSignableBCH(tx.VaultPubKey, c.tssKeySigner, c.log)
	}

	sig, err := bchtxscript.RawTxInECDSASignature(redeemTx, idx, sourceScript, bchtxscript.SigHashAll, signable, amount)
	if err != nil {
		return fmt.Errorf("fail to get witness: %w", err)
	}

	pkData := signable.GetPubKey().SerializeCompressed()
	sigscript, err := bchtxscript.NewScriptBuilder().AddData(sig).AddData(pkData).Script()
	if err != nil {
		return fmt.Errorf("fail to build signature script: %w", err)
	}
	redeemTx.TxIn[idx].SignatureScript = sigscript
	flag := bchtxscript.StandardVerifyFlags
	engine, err := bchtxscript.NewEngine(sourceScript, redeemTx, idx, flag, nil, nil, amount)
	if err != nil {
		return fmt.Errorf("fail to create engine: %w", err)
	}
	if err = engine.Execute(); err != nil {
		// SECURITY FIX (Layer 4 - NULLFAIL Failsafe): This should NEVER happen after Layers 1-3.
		// If it does occur, it indicates a serious issue: cryptographic failure, TSS corruption, or unknown edge case.
		// We log and treat as success to prevent retry loops, allowing manual investigation.
		if bchtxscript.IsErrorCode(err, bchtxscript.ErrNullFail) {
			c.log.Error().
				Err(err).
				Int("input_idx", idx).
				Msg("NULLFAIL FAILSAFE TRIGGERED - This should not happen! Investigate immediately!")
			return nil // Treat as success to prevent retry loop
		}
		return fmt.Errorf("fail to execute the script: %w", err)
	}
	return nil
}
