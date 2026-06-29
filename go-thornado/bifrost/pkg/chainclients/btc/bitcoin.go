package btc

import (
	"bytes"
	"context"
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
	btctxscript "github.com/thornadocash/go-thornado/bifrost/txscript/txscript"

	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
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

type schnorrSourceScriptCacheKey struct {
	pubkey    common.PubKey
	pathIndex uint64
}

func (c *Client) getSchnorrSourceScriptAtPath(pubkey common.PubKey, pathIndex uint64) ([]byte, error) {
	key := schnorrSourceScriptCacheKey{
		pubkey:    pubkey,
		pathIndex: pathIndex,
	}
	if cached, ok := c.sourceScriptCache.Load(key); ok {
		return append([]byte(nil), cached.([]byte)...), nil
	}

	taprootKey, err := common.DeriveBTCTaprootPubKey(pubkey, pathIndex)
	if err != nil {
		return nil, err
	}
	script := append([]byte{0x51, 0x20}, taprootKey...)
	if parsedTaprootKey, err := btcschnorr.ParsePubKey(taprootKey); err == nil {
		c.taprootPubKeyCache.Store(key, parsedTaprootKey)
	}
	c.sourceScriptCache.Store(key, script)
	return append([]byte(nil), script...), nil
}

func (c *Client) taprootUTXOWitness(ctx context.Context, redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) (btcwire.TxWitness, error) {
	sigHash, err := taprootKeySpendSigHash(redeemTx, idx, amount, sourceScript)
	if err != nil {
		return nil, fmt.Errorf("fail to get taproot sighash: %w", err)
	}

	var (
		sig []byte
	)
	if signer, ok := c.frostKeySigner.(interface {
		RemoteSignWithPathContext(context.Context, []byte, common.SigningAlgo, string, uint64) ([]byte, []byte, error)
	}); ok {
		sig, _, err = signer.RemoteSignWithPathContext(ctx, sigHash, common.SigningAlgoSecp256k1, tx.VaultPubKey.String(), tx.VaultPathIndex)
	} else if signer, ok := c.frostKeySigner.(interface {
		RemoteSignWithPath([]byte, common.SigningAlgo, string, uint64) ([]byte, []byte, error)
	}); ok {
		sig, _, err = signer.RemoteSignWithPath(sigHash, common.SigningAlgoSecp256k1, tx.VaultPubKey.String(), tx.VaultPathIndex)
	} else {
		sig, _, err = c.frostKeySigner.RemoteSign(sigHash, common.SigningAlgoSecp256k1, tx.VaultPubKey.String())
	}
	if err != nil {
		return nil, fmt.Errorf("fail to frost schnorr sign: %w", err)
	}
	if len(sig) != btcschnorr.SignatureSize {
		return nil, fmt.Errorf("invalid schnorr signature length: %d", len(sig))
	}
	sig = append(sig, schnorrSigHashAllAnyoneCanPay)

	taprootKey, err := c.taprootWitnessVerifyPubKey(tx, sourceScript)
	if err != nil {
		return nil, err
	}
	parsedSig, err := btcschnorr.ParseSignature(sig[:btcschnorr.SignatureSize])
	if err != nil {
		return nil, fmt.Errorf("fail to parse schnorr signature: %w", err)
	}
	if !parsedSig.Verify(sigHash, taprootKey) {
		return nil, fmt.Errorf("schnorr signature failed local verification")
	}

	return btcwire.TxWitness{sig}, nil
}

func (c *Client) taprootWitnessVerifyPubKey(tx stypes.TxOutItem, sourceScript []byte) (*btcec.PublicKey, error) {
	key := schnorrSourceScriptCacheKey{
		pubkey:    tx.VaultPubKey,
		pathIndex: tx.VaultPathIndex,
	}
	if cached, ok := c.taprootPubKeyCache.Load(key); ok {
		return cached.(*btcec.PublicKey), nil
	}

	pubKeyBytes, err := common.DeriveBTCTaprootPubKey(tx.VaultPubKey, tx.VaultPathIndex)
	if err != nil {
		return nil, err
	}
	taprootKey, err := btcschnorr.ParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("fail to parse schnorr public key: %w", err)
	}
	c.taprootPubKeyCache.Store(key, taprootKey)
	return taprootKey, nil
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
