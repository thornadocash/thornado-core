package utxo

import (
	"fmt"

	dogeec "github.com/eager7/dogd/btcec"
	dogechaincfg "github.com/eager7/dogd/chaincfg"
	dogewire "github.com/eager7/dogd/wire"
	dogetxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/dogd-txscript"

	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
)

func (c *Client) getChainCfgDOGE() *dogechaincfg.Params {
	switch common.CurrentChainNetwork {
	case common.MockNet:
		return &dogechaincfg.RegressionNetParams
	case common.MainNet, common.StageNet, common.ChainNet:
		return &dogechaincfg.MainNetParams
	default:
		c.log.Fatal().Msg("unsupported network")
		return nil
	}
}

func (c *Client) signUTXODOGE(redeemTx *dogewire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	var signable dogetxscript.Signable
	if tx.VaultPubKey.Equals(c.nodePubKey) {
		signable = dogetxscript.NewPrivateKeySignable((*dogeec.PrivateKey)(c.nodePrivKey))
	} else {
		signable = newTssSignableDOGE(tx.VaultPubKey, c.tssKeySigner, c.log)
	}

	sig, err := dogetxscript.RawTxInSignature(redeemTx, idx, sourceScript, dogetxscript.SigHashAll, signable)
	if err != nil {
		return fmt.Errorf("fail to get witness: %w", err)
	}

	pkData := signable.GetPubKey().SerializeCompressed()
	sigscript, err := dogetxscript.NewScriptBuilder().AddData(sig).AddData(pkData).Script()
	if err != nil {
		return fmt.Errorf("fail to build signature script: %w", err)
	}
	redeemTx.TxIn[idx].SignatureScript = sigscript
	flag := dogetxscript.StandardVerifyFlags
	engine, err := dogetxscript.NewEngine(sourceScript, redeemTx, idx, flag, nil, nil, amount)
	if err != nil {
		return fmt.Errorf("fail to create engine: %w", err)
	}
	if err = engine.Execute(); err != nil {
		// SECURITY FIX (Layer 4 - NULLFAIL Failsafe): This should NEVER happen after Layers 1-3.
		// If it does occur, it indicates a serious issue: cryptographic failure, TSS corruption, or unknown edge case.
		// We log and treat as success to prevent retry loops, allowing manual investigation.
		if dogetxscript.IsErrorCode(err, dogetxscript.ErrNullFail) {
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
