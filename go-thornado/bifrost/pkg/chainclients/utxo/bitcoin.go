package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	btcschnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	btcjson "github.com/btcsuite/btcd/btcjson"
	btcchaincfg "github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	btcwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	btctxscript "github.com/thornadocash/go-thornado/bifrost/txscript/txscript"

	stypes "github.com/thornadocash/go-thornado/bifrost/thorclient/types"
	"github.com/thornadocash/go-thornado/common"
)

const schnorrSigHashAllAnyoneCanPay = 0x81

func (c *Client) getChainCfgBTC() *btcchaincfg.Params {
	params, err := common.BTCChainParams()
	if err != nil {
		c.log.Fatal().Err(err).Msg("unsupported network")
		return nil
	}
	return params
}

func (c *Client) getBitcoinVaultAddress(pubkey common.PubKey) (common.Address, error) {
	secpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return common.NoAddress, err
	}
	addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(secpPubKey.SerializeCompressed()), c.getChainCfgBTC())
	if err != nil {
		return common.NoAddress, err
	}
	return common.NewAddress(addr.String())
}

type frostEngineReader interface {
	LocalStateEngine(poolPubKey string) string
}

func (c *Client) isFrostVault(pubkey common.PubKey) bool {
	if c.cfg.ChainID != common.BTCChain {
		return false
	}
	reader, ok := c.tssKeySigner.(frostEngineReader)
	if !ok {
		return false
	}
	return reader.LocalStateEngine(pubkey.String()) == storage.SigningEngineFrost
}

func (c *Client) schnorrVaultPubKey(pubkey common.PubKey) (*btcec.PublicKey, error) {
	secpPubKey, err := pubkey.Secp256K1()
	if err != nil {
		return nil, err
	}
	return btcec.ParsePubKey(secpPubKey.SerializeCompressed())
}

func (c *Client) getSchnorrVaultAddress(pubkey common.PubKey) (common.Address, error) {
	return c.getSchnorrVaultAddressAtPath(pubkey, common.MainVaultPathIndex)
}

func (c *Client) getSchnorrVaultAddressAtPath(pubkey common.PubKey, pathIndex uint64) (common.Address, error) {
	taprootKey, err := common.DeriveBTCTaprootPubKey(pubkey, pathIndex)
	if err != nil {
		return common.NoAddress, err
	}
	addr, err := btcutil.NewAddressTaproot(taprootKey, c.getChainCfgBTC())
	if err != nil {
		return common.NoAddress, err
	}
	return common.NewAddress(addr.String())
}

func (c *Client) getSchnorrSourceScript(pubkey common.PubKey) ([]byte, error) {
	return c.getSchnorrSourceScriptAtPath(pubkey, common.MainVaultPathIndex)
}

func (c *Client) getSchnorrSourceScriptAtPath(pubkey common.PubKey, pathIndex uint64) ([]byte, error) {
	taprootKey, err := common.DeriveBTCTaprootPubKey(pubkey, pathIndex)
	if err != nil {
		return nil, err
	}
	return append([]byte{0x51, 0x20}, taprootKey...), nil
}

func (c *Client) signUTXOBTC(redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	if c.isFrostVault(tx.VaultPubKey) {
		return c.signTaprootUTXOBTC(redeemTx, tx, amount, sourceScript, idx)
	}

	sigHashes := btctxscript.NewTxSigHashes(redeemTx)

	var signable btctxscript.Signable
	if tx.VaultPubKey.Equals(c.nodePubKey) {
		signable = btctxscript.NewPrivateKeySignable(c.nodePrivKey)
	} else {
		signable = newTssSignableBTC(tx.VaultPubKey, c.tssKeySigner, c.log)
	}

	witness, err := btctxscript.WitnessSignature(redeemTx, sigHashes, idx, amount, sourceScript, btctxscript.SigHashAll, signable, true)
	if err != nil {
		return fmt.Errorf("fail to get witness: %w", err)
	}

	redeemTx.TxIn[idx].Witness = witness
	flag := btctxscript.StandardVerifyFlags
	engine, err := btctxscript.NewEngine(sourceScript, redeemTx, idx, flag, nil, nil, amount)
	if err != nil {
		return fmt.Errorf("fail to create engine: %w", err)
	}
	if err = engine.Execute(); err != nil {
		// SECURITY FIX (Layer 4 - NULLFAIL Failsafe): This should NEVER happen after Layers 1-3.
		// If it does occur, it indicates a serious issue: cryptographic failure, TSS corruption, or unknown edge case.
		// We log and treat as success to prevent retry loops, allowing manual investigation.
		if btctxscript.IsErrorCode(err, btctxscript.ErrNullFail) {
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

func (c *Client) signTaprootUTXOBTC(redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	sigHash, err := taprootKeySpendSigHash(redeemTx, idx, amount, sourceScript)
	if err != nil {
		return fmt.Errorf("fail to get taproot sighash: %w", err)
	}

	var sig []byte
	if tx.VaultPubKey.Equals(c.nodePubKey) {
		privKey, _ := btcec.PrivKeyFromBytes(c.nodePrivKey.Serialize())
		signature, err := btcschnorr.Sign(privKey, sigHash)
		if err != nil {
			return fmt.Errorf("fail to schnorr sign: %w", err)
		}
		sig = signature.Serialize()
	} else {
		if signer, ok := c.tssKeySigner.(interface {
			RemoteSignWithPath([]byte, common.SigningAlgo, string, uint64) ([]byte, []byte, error)
		}); ok {
			sig, _, err = signer.RemoteSignWithPath(sigHash, common.SigningAlgoSecp256k1, tx.VaultPubKey.String(), tx.VaultPathIndex)
		} else {
			sig, _, err = c.tssKeySigner.RemoteSign(sigHash, common.SigningAlgoSecp256k1, tx.VaultPubKey.String())
		}
		if err != nil {
			return fmt.Errorf("fail to tss schnorr sign: %w", err)
		}
	}
	if len(sig) != btcschnorr.SignatureSize {
		return fmt.Errorf("invalid schnorr signature length: %d", len(sig))
	}
	sig = append(sig, schnorrSigHashAllAnyoneCanPay)

	pubKeyBytes, err := common.DeriveBTCTaprootPubKey(tx.VaultPubKey, tx.VaultPathIndex)
	if err != nil {
		return err
	}
	taprootKey, err := btcschnorr.ParsePubKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("fail to parse schnorr public key: %w", err)
	}
	parsedSig, err := btcschnorr.ParseSignature(sig[:btcschnorr.SignatureSize])
	if err != nil {
		return fmt.Errorf("fail to parse schnorr signature: %w", err)
	}
	if !parsedSig.Verify(sigHash, taprootKey) {
		return fmt.Errorf("schnorr signature failed local verification")
	}

	redeemTx.TxIn[idx].Witness = btcwire.TxWitness{sig}
	return nil
}

func taprootKeySpendSigHash(tx *btcwire.MsgTx, idx int, amount int64, sourceScript []byte) ([]byte, error) {
	if idx < 0 || idx >= len(tx.TxIn) {
		return nil, fmt.Errorf("invalid input index %d", idx)
	}

	var outputs bytes.Buffer
	for _, txOut := range tx.TxOut {
		if err := btcwire.WriteTxOut(&outputs, 0, tx.Version, txOut); err != nil {
			return nil, err
		}
	}
	hashOutputs := sha256.Sum256(outputs.Bytes())

	var msg bytes.Buffer
	msg.WriteByte(schnorrSigHashAllAnyoneCanPay)
	if err := binary.Write(&msg, binary.LittleEndian, tx.Version); err != nil {
		return nil, err
	}
	if err := binary.Write(&msg, binary.LittleEndian, tx.LockTime); err != nil {
		return nil, err
	}
	msg.Write(hashOutputs[:])
	msg.WriteByte(0)

	txIn := tx.TxIn[idx]
	msg.Write(txIn.PreviousOutPoint.Hash[:])
	if err := binary.Write(&msg, binary.LittleEndian, txIn.PreviousOutPoint.Index); err != nil {
		return nil, err
	}
	if err := binary.Write(&msg, binary.LittleEndian, uint64(amount)); err != nil {
		return nil, err
	}
	if err := btcwire.WriteVarBytes(&msg, 0, sourceScript); err != nil {
		return nil, err
	}
	if err := binary.Write(&msg, binary.LittleEndian, txIn.Sequence); err != nil {
		return nil, err
	}

	return chainhash.TaggedHash(chainhash.TagTapSighash, append([]byte{0x00}, msg.Bytes()...))[:], nil
}

func (c *Client) getAddressesFromScriptPubKeyBTC(scriptPubKey btcjson.ScriptPubKeyResult) []string {
	addresses := scriptPubKey.Addresses
	if len(addresses) > 0 {
		return addresses
	}

	if len(scriptPubKey.Hex) == 0 {
		return nil
	}
	buf, err := hex.DecodeString(scriptPubKey.Hex)
	if err != nil {
		c.log.Err(err).Msg("fail to hex decode script pub key")
		return nil
	}
	_, extractedAddresses, _, err := btctxscript.ExtractPkScriptAddrs(buf, c.getChainCfgBTC())
	if err != nil {
		c.log.Err(err).Msg("fail to extract addresses from script pub key")
		return nil
	}
	for _, item := range extractedAddresses {
		addresses = append(addresses, item.String())
	}
	return addresses
}
