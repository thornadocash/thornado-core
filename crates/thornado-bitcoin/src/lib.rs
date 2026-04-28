use bitcoin::absolute::LockTime;
use bitcoin::consensus::encode::{serialize, serialize_hex};
use bitcoin::{
    Address, Amount, Network, OutPoint, ScriptBuf, Sequence, Transaction, TxIn, TxOut, Txid,
    Witness,
};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};
use std::io::{Read, Write};
use std::net::TcpStream;
use std::str::FromStr;

pub type Result<T> = std::result::Result<T, Error>;

pub const DEFAULT_FEE_RATE_SATS_PER_VB: u64 = 2;
pub const DEFAULT_MAX_FEE_RATE_SATS_PER_VB: u64 = 200;
pub const DEFAULT_MAX_INPUTS: usize = 15;
pub const DEFAULT_MIN_UTXO_CONFIRMATIONS: u64 = 1;
pub const DEFAULT_MAX_MEMPOOL_CHAIN_LENGTH: u64 = 25;
pub const DEFAULT_CONSOLIDATION_MIN_UTXOS: usize = 15;
pub const CHANGE_DUST_SATS: u64 = 546;

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum Error {
    #[error("bitcoin adapter is regtest only")]
    NonRegtestNetwork,
    #[error("invalid txid: {0}")]
    InvalidTxid(String),
    #[error("invalid script hex: {0}")]
    InvalidScript(String),
    #[error("invalid recipient address: {0}")]
    InvalidAddress(String),
    #[error("no spendable UTXOs")]
    NoUtxos,
    #[error("insufficient funds: need {needed_sats} sats, have {available_sats} sats")]
    InsufficientFunds {
        needed_sats: u64,
        available_sats: u64,
    },
    #[error("withdrawal amount must be above dust")]
    DustWithdrawal,
    #[error("invalid or unsupported UTXO script")]
    InvalidUtxoScript,
    #[error("bitcoin withdrawal already exists")]
    WithdrawalAlreadyBuilt,
    #[error("bitcoin withdrawal not found")]
    WithdrawalNotFound,
    #[error("bitcoin consolidation already exists")]
    ConsolidationAlreadyBuilt,
    #[error("bitcoin consolidation not found")]
    ConsolidationNotFound,
    #[error("UTXO is already reserved")]
    UtxoReserved,
    #[error("bitcoin signing checkpoint does not match the built transaction")]
    CheckpointMismatch,
    #[error("bitcoin signing checkpoint spends unavailable UTXOs")]
    CheckpointInputsUnavailable,
    #[error("not enough UTXOs to consolidate")]
    NothingToConsolidate,
    #[error("bitcoin RPC error: {0}")]
    Rpc(String),
    #[error("operation is only supported by the dev bitcoin backend")]
    DevOnly,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RegtestUtxo {
    pub txid: String,
    pub vout: u32,
    pub value_sats: u64,
    pub script_pubkey_hex: String,
    #[serde(default = "default_confirmations")]
    pub confirmations: u64,
    #[serde(default)]
    pub is_self_transfer: bool,
    #[serde(default)]
    pub mempool_ancestor_count: u64,
    #[serde(default)]
    pub mempool_descendant_count: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinWithdrawalRequest {
    pub withdrawal_id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_rate_sats_per_vb: u64,
    pub change_script_pubkey_hex: Option<String>,
    #[serde(default)]
    pub max_fee_rate_sats_per_vb: Option<u64>,
    #[serde(default)]
    pub min_relay_fee_sats: Option<u64>,
    #[serde(default)]
    pub max_inputs: Option<usize>,
    #[serde(default)]
    pub min_confirmations: Option<u64>,
    #[serde(default)]
    pub max_mempool_chain_length: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BuiltWithdrawal {
    pub withdrawal_id: String,
    pub unsigned_tx_hex: String,
    pub input_value_sats: u64,
    pub output_value_sats: u64,
    pub change_value_sats: u64,
    pub miner_fee_sats: u64,
    pub selected_utxos: Vec<RegtestUtxo>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinConsolidationRequest {
    pub consolidation_id: String,
    pub fee_rate_sats_per_vb: u64,
    pub change_script_pubkey_hex: String,
    #[serde(default)]
    pub min_utxos: Option<usize>,
    #[serde(default)]
    pub max_inputs: Option<usize>,
    #[serde(default)]
    pub min_confirmations: Option<u64>,
    #[serde(default)]
    pub max_mempool_chain_length: Option<u64>,
    #[serde(default)]
    pub max_fee_rate_sats_per_vb: Option<u64>,
    #[serde(default)]
    pub min_relay_fee_sats: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BuiltConsolidation {
    pub consolidation_id: String,
    pub unsigned_tx_hex: String,
    pub input_value_sats: u64,
    pub output_value_sats: u64,
    pub miner_fee_sats: u64,
    pub selected_utxos: Vec<RegtestUtxo>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinSolvencyReport {
    pub expected_sats: u64,
    pub actual_sats: u64,
    pub confirmed_sats: u64,
    pub self_mempool_sats: u64,
    pub external_mempool_sats: u64,
    pub spendable_utxo_count: usize,
    pub reserved_sats: u64,
    pub solvent: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SigningCheckpointValidation {
    pub withdrawal_id: String,
    pub txid: String,
    pub input_count: usize,
    pub valid: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct RegtestVault {
    pub utxos: Vec<RegtestUtxo>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinWithdrawalRecord {
    pub built: BuiltWithdrawal,
    pub broadcast_txid: Option<String>,
    pub confirmed_height: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinConsolidationRecord {
    pub built: BuiltConsolidation,
    pub broadcast_txid: Option<String>,
    pub confirmed_height: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinRpcConfig {
    pub url: String,
    pub user: String,
    pub password: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct DevBitcoinBackend {
    vault: RegtestVault,
    built_withdrawals: BTreeMap<String, BitcoinWithdrawalRecord>,
    built_consolidations: BTreeMap<String, BitcoinConsolidationRecord>,
    reserved_outpoints: BTreeSet<String>,
    self_transactions: BTreeSet<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RpcBitcoinBackend {
    config: BitcoinRpcConfig,
    built_withdrawals: BTreeMap<String, BitcoinWithdrawalRecord>,
    built_consolidations: BTreeMap<String, BitcoinConsolidationRecord>,
    reserved_outpoints: BTreeSet<String>,
    self_transactions: BTreeSet<String>,
}

pub trait BitcoinBackend {
    fn import_dev_utxo(&mut self, utxo: RegtestUtxo) -> Result<()>;
    fn list_utxos(&self) -> Vec<RegtestUtxo>;
    fn build_withdrawal(&mut self, request: BitcoinWithdrawalRequest) -> Result<BuiltWithdrawal>;
    fn get_withdrawal(&self, withdrawal_id: &str) -> Result<BitcoinWithdrawalRecord>;
    fn mark_broadcast(
        &mut self,
        withdrawal_id: &str,
        txid: String,
    ) -> Result<BitcoinWithdrawalRecord>;
    fn mark_confirmed(
        &mut self,
        withdrawal_id: &str,
        height: u64,
    ) -> Result<BitcoinWithdrawalRecord>;
    fn broadcast_withdrawal(
        &mut self,
        withdrawal_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinWithdrawalRecord>;
    fn validate_signing_checkpoint(
        &self,
        withdrawal_id: &str,
        unsigned_tx_hex: String,
    ) -> Result<SigningCheckpointValidation>;
    fn report_solvency(&self, expected_sats: u64) -> Result<BitcoinSolvencyReport>;
    fn build_consolidation(
        &mut self,
        request: BitcoinConsolidationRequest,
    ) -> Result<BuiltConsolidation>;
    fn get_consolidation(&self, consolidation_id: &str) -> Result<BitcoinConsolidationRecord>;
    fn broadcast_consolidation(
        &mut self,
        consolidation_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinConsolidationRecord>;
}

impl DevBitcoinBackend {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn records(&self) -> &BTreeMap<String, BitcoinWithdrawalRecord> {
        &self.built_withdrawals
    }
}

impl BitcoinBackend for DevBitcoinBackend {
    fn import_dev_utxo(&mut self, utxo: RegtestUtxo) -> Result<()> {
        self.vault.import_utxo(utxo)
    }

    fn list_utxos(&self) -> Vec<RegtestUtxo> {
        self.vault.utxos.clone()
    }

    fn build_withdrawal(&mut self, request: BitcoinWithdrawalRequest) -> Result<BuiltWithdrawal> {
        if self.built_withdrawals.contains_key(&request.withdrawal_id) {
            return Err(Error::WithdrawalAlreadyBuilt);
        }
        if self
            .built_consolidations
            .contains_key(&request.withdrawal_id)
        {
            return Err(Error::WithdrawalAlreadyBuilt);
        }

        let spendable = self
            .vault
            .clone_with_excluded_outpoints(&self.reserved_outpoints);
        let built = spendable.build_withdrawal(request)?;
        for utxo in &built.selected_utxos {
            let outpoint = outpoint_key(utxo);
            if !self.reserved_outpoints.insert(outpoint) {
                return Err(Error::UtxoReserved);
            }
        }

        self.built_withdrawals.insert(
            built.withdrawal_id.clone(),
            BitcoinWithdrawalRecord {
                built: built.clone(),
                broadcast_txid: None,
                confirmed_height: None,
            },
        );
        Ok(built)
    }

    fn get_withdrawal(&self, withdrawal_id: &str) -> Result<BitcoinWithdrawalRecord> {
        self.built_withdrawals
            .get(withdrawal_id)
            .cloned()
            .ok_or(Error::WithdrawalNotFound)
    }

    fn mark_broadcast(
        &mut self,
        withdrawal_id: &str,
        txid: String,
    ) -> Result<BitcoinWithdrawalRecord> {
        Txid::from_str(&txid).map_err(|e| Error::InvalidTxid(e.to_string()))?;
        self.self_transactions.insert(txid.clone());
        let record = self
            .built_withdrawals
            .get_mut(withdrawal_id)
            .ok_or(Error::WithdrawalNotFound)?;
        record.broadcast_txid = Some(txid);
        Ok(record.clone())
    }

    fn mark_confirmed(
        &mut self,
        withdrawal_id: &str,
        height: u64,
    ) -> Result<BitcoinWithdrawalRecord> {
        let record = self
            .built_withdrawals
            .get_mut(withdrawal_id)
            .ok_or(Error::WithdrawalNotFound)?;
        record.confirmed_height = Some(height);
        Ok(record.clone())
    }

    fn broadcast_withdrawal(
        &mut self,
        withdrawal_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinWithdrawalRecord> {
        let record = self.get_withdrawal(withdrawal_id)?;
        validate_checkpoint_inputs(&record.built.unsigned_tx_hex, &self.vault.utxos)?;
        let tx = validate_signed_tx_matches_built(&record.built.unsigned_tx_hex, &signed_tx_hex)?;
        let txid = tx.compute_txid().to_string();
        self.mark_broadcast(withdrawal_id, txid)
    }

    fn validate_signing_checkpoint(
        &self,
        withdrawal_id: &str,
        unsigned_tx_hex: String,
    ) -> Result<SigningCheckpointValidation> {
        let record = self.get_withdrawal(withdrawal_id)?;
        validate_checkpoint_match(&record.built.unsigned_tx_hex, &unsigned_tx_hex)?;
        validate_checkpoint_inputs(&unsigned_tx_hex, &self.vault.utxos)?;
        let tx = transaction_from_hex(&unsigned_tx_hex)?;
        Ok(SigningCheckpointValidation {
            withdrawal_id: withdrawal_id.to_string(),
            txid: tx.compute_txid().to_string(),
            input_count: tx.input.len(),
            valid: true,
        })
    }

    fn report_solvency(&self, expected_sats: u64) -> Result<BitcoinSolvencyReport> {
        Ok(solvency_report(
            expected_sats,
            &self.vault.utxos,
            &self.reserved_outpoints,
        ))
    }

    fn build_consolidation(
        &mut self,
        request: BitcoinConsolidationRequest,
    ) -> Result<BuiltConsolidation> {
        if self
            .built_consolidations
            .contains_key(&request.consolidation_id)
            || self
                .built_withdrawals
                .contains_key(&request.consolidation_id)
        {
            return Err(Error::ConsolidationAlreadyBuilt);
        }
        let spendable = self
            .vault
            .clone_with_excluded_outpoints(&self.reserved_outpoints);
        let built = spendable.build_consolidation(request)?;
        for utxo in &built.selected_utxos {
            if !self.reserved_outpoints.insert(outpoint_key(utxo)) {
                return Err(Error::UtxoReserved);
            }
        }
        self.built_consolidations.insert(
            built.consolidation_id.clone(),
            BitcoinConsolidationRecord {
                built: built.clone(),
                broadcast_txid: None,
                confirmed_height: None,
            },
        );
        Ok(built)
    }

    fn get_consolidation(&self, consolidation_id: &str) -> Result<BitcoinConsolidationRecord> {
        self.built_consolidations
            .get(consolidation_id)
            .cloned()
            .ok_or(Error::ConsolidationNotFound)
    }

    fn broadcast_consolidation(
        &mut self,
        consolidation_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinConsolidationRecord> {
        let record = self.get_consolidation(consolidation_id)?;
        validate_checkpoint_inputs(&record.built.unsigned_tx_hex, &self.vault.utxos)?;
        let tx = validate_signed_tx_matches_built(&record.built.unsigned_tx_hex, &signed_tx_hex)?;
        let txid = tx.compute_txid().to_string();
        self.self_transactions.insert(txid.clone());
        let record = self
            .built_consolidations
            .get_mut(consolidation_id)
            .ok_or(Error::ConsolidationNotFound)?;
        record.broadcast_txid = Some(txid);
        Ok(record.clone())
    }
}

impl RpcBitcoinBackend {
    pub fn new(config: BitcoinRpcConfig) -> Result<Self> {
        let backend = Self {
            config,
            built_withdrawals: BTreeMap::new(),
            built_consolidations: BTreeMap::new(),
            reserved_outpoints: BTreeSet::new(),
            self_transactions: BTreeSet::new(),
        };
        backend.ensure_regtest()?;
        Ok(backend)
    }

    pub fn config(&self) -> &BitcoinRpcConfig {
        &self.config
    }

    fn ensure_regtest(&self) -> Result<()> {
        let info = self.rpc("getblockchaininfo", serde_json::json!([]))?;
        let chain = info
            .get("chain")
            .and_then(serde_json::Value::as_str)
            .ok_or_else(|| Error::Rpc("getblockchaininfo response missing chain".to_string()))?;
        if chain == "regtest" {
            Ok(())
        } else {
            Err(Error::NonRegtestNetwork)
        }
    }

    fn rpc(&self, method: &str, params: serde_json::Value) -> Result<serde_json::Value> {
        let request = serde_json::json!({
            "jsonrpc": "1.0",
            "id": "thornado",
            "method": method,
            "params": params,
        });
        let response = rpc_http_post(
            &self.config.url,
            &self.config.user,
            &self.config.password,
            &request.to_string(),
        )?;
        let value: serde_json::Value =
            serde_json::from_str(&response).map_err(|e| Error::Rpc(e.to_string()))?;
        if !value
            .get("error")
            .unwrap_or(&serde_json::Value::Null)
            .is_null()
        {
            return Err(Error::Rpc(value["error"].to_string()));
        }
        value
            .get("result")
            .cloned()
            .ok_or_else(|| Error::Rpc("RPC response missing result".to_string()))
    }

    fn rpc_utxos(&self) -> Result<Vec<RegtestUtxo>> {
        let mut utxos = rpc_utxos_from_value(&self.rpc("listunspent", serde_json::json!([0]))?)?;
        for utxo in &mut utxos {
            utxo.is_self_transfer = self.self_transactions.contains(&utxo.txid);
            if utxo.confirmations == 0 {
                if let Ok(entry) = self.rpc("getmempoolentry", serde_json::json!([utxo.txid])) {
                    utxo.mempool_ancestor_count = entry
                        .get("ancestorcount")
                        .and_then(serde_json::Value::as_u64)
                        .unwrap_or(utxo.mempool_ancestor_count);
                    utxo.mempool_descendant_count = entry
                        .get("descendantcount")
                        .and_then(serde_json::Value::as_u64)
                        .unwrap_or(utxo.mempool_descendant_count);
                }
            }
        }
        Ok(utxos)
    }
}

impl BitcoinBackend for RpcBitcoinBackend {
    fn import_dev_utxo(&mut self, _utxo: RegtestUtxo) -> Result<()> {
        Err(Error::DevOnly)
    }

    fn list_utxos(&self) -> Vec<RegtestUtxo> {
        self.rpc_utxos().unwrap_or_default()
    }

    fn build_withdrawal(&mut self, request: BitcoinWithdrawalRequest) -> Result<BuiltWithdrawal> {
        if self.built_withdrawals.contains_key(&request.withdrawal_id) {
            return Err(Error::WithdrawalAlreadyBuilt);
        }
        if self
            .built_consolidations
            .contains_key(&request.withdrawal_id)
        {
            return Err(Error::WithdrawalAlreadyBuilt);
        }

        let mut vault = RegtestVault::default();
        for utxo in self.rpc_utxos()? {
            if !self.reserved_outpoints.contains(&outpoint_key(&utxo)) {
                vault.import_utxo(utxo)?;
            }
        }
        let built = vault.build_withdrawal(request)?;
        for utxo in &built.selected_utxos {
            if !self.reserved_outpoints.insert(outpoint_key(utxo)) {
                return Err(Error::UtxoReserved);
            }
        }
        self.built_withdrawals.insert(
            built.withdrawal_id.clone(),
            BitcoinWithdrawalRecord {
                built: built.clone(),
                broadcast_txid: None,
                confirmed_height: None,
            },
        );
        Ok(built)
    }

    fn get_withdrawal(&self, withdrawal_id: &str) -> Result<BitcoinWithdrawalRecord> {
        self.built_withdrawals
            .get(withdrawal_id)
            .cloned()
            .ok_or(Error::WithdrawalNotFound)
    }

    fn mark_broadcast(
        &mut self,
        withdrawal_id: &str,
        txid: String,
    ) -> Result<BitcoinWithdrawalRecord> {
        Txid::from_str(&txid).map_err(|e| Error::InvalidTxid(e.to_string()))?;
        self.self_transactions.insert(txid.clone());
        let record = self
            .built_withdrawals
            .get_mut(withdrawal_id)
            .ok_or(Error::WithdrawalNotFound)?;
        record.broadcast_txid = Some(txid);
        Ok(record.clone())
    }

    fn mark_confirmed(
        &mut self,
        withdrawal_id: &str,
        height: u64,
    ) -> Result<BitcoinWithdrawalRecord> {
        let record = self
            .built_withdrawals
            .get_mut(withdrawal_id)
            .ok_or(Error::WithdrawalNotFound)?;
        record.confirmed_height = Some(height);
        Ok(record.clone())
    }

    fn broadcast_withdrawal(
        &mut self,
        withdrawal_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinWithdrawalRecord> {
        let record = self.get_withdrawal(withdrawal_id)?;
        validate_checkpoint_inputs(&record.built.unsigned_tx_hex, &self.rpc_utxos()?)?;
        validate_signed_tx_matches_built(&record.built.unsigned_tx_hex, &signed_tx_hex)?;
        let txid = self
            .rpc("sendrawtransaction", serde_json::json!([signed_tx_hex]))?
            .as_str()
            .ok_or_else(|| Error::Rpc("sendrawtransaction result was not a txid".to_string()))?
            .to_string();
        self.mark_broadcast(withdrawal_id, txid)
    }

    fn validate_signing_checkpoint(
        &self,
        withdrawal_id: &str,
        unsigned_tx_hex: String,
    ) -> Result<SigningCheckpointValidation> {
        let record = self.get_withdrawal(withdrawal_id)?;
        validate_checkpoint_match(&record.built.unsigned_tx_hex, &unsigned_tx_hex)?;
        validate_checkpoint_inputs(&unsigned_tx_hex, &self.rpc_utxos()?)?;
        let tx = transaction_from_hex(&unsigned_tx_hex)?;
        Ok(SigningCheckpointValidation {
            withdrawal_id: withdrawal_id.to_string(),
            txid: tx.compute_txid().to_string(),
            input_count: tx.input.len(),
            valid: true,
        })
    }

    fn report_solvency(&self, expected_sats: u64) -> Result<BitcoinSolvencyReport> {
        Ok(solvency_report(
            expected_sats,
            &self.rpc_utxos()?,
            &self.reserved_outpoints,
        ))
    }

    fn build_consolidation(
        &mut self,
        request: BitcoinConsolidationRequest,
    ) -> Result<BuiltConsolidation> {
        if self
            .built_consolidations
            .contains_key(&request.consolidation_id)
            || self
                .built_withdrawals
                .contains_key(&request.consolidation_id)
        {
            return Err(Error::ConsolidationAlreadyBuilt);
        }
        let mut vault = RegtestVault::default();
        for utxo in self.rpc_utxos()? {
            if !self.reserved_outpoints.contains(&outpoint_key(&utxo)) {
                vault.import_utxo(utxo)?;
            }
        }
        let built = vault.build_consolidation(request)?;
        for utxo in &built.selected_utxos {
            if !self.reserved_outpoints.insert(outpoint_key(utxo)) {
                return Err(Error::UtxoReserved);
            }
        }
        self.built_consolidations.insert(
            built.consolidation_id.clone(),
            BitcoinConsolidationRecord {
                built: built.clone(),
                broadcast_txid: None,
                confirmed_height: None,
            },
        );
        Ok(built)
    }

    fn get_consolidation(&self, consolidation_id: &str) -> Result<BitcoinConsolidationRecord> {
        self.built_consolidations
            .get(consolidation_id)
            .cloned()
            .ok_or(Error::ConsolidationNotFound)
    }

    fn broadcast_consolidation(
        &mut self,
        consolidation_id: &str,
        signed_tx_hex: String,
    ) -> Result<BitcoinConsolidationRecord> {
        let record = self.get_consolidation(consolidation_id)?;
        validate_checkpoint_inputs(&record.built.unsigned_tx_hex, &self.rpc_utxos()?)?;
        validate_signed_tx_matches_built(&record.built.unsigned_tx_hex, &signed_tx_hex)?;
        let txid = self
            .rpc("sendrawtransaction", serde_json::json!([signed_tx_hex]))?
            .as_str()
            .ok_or_else(|| Error::Rpc("sendrawtransaction result was not a txid".to_string()))?
            .to_string();
        Txid::from_str(&txid).map_err(|e| Error::InvalidTxid(e.to_string()))?;
        self.self_transactions.insert(txid.clone());
        let record = self
            .built_consolidations
            .get_mut(consolidation_id)
            .ok_or(Error::ConsolidationNotFound)?;
        record.broadcast_txid = Some(txid);
        Ok(record.clone())
    }
}

impl RegtestVault {
    pub fn import_utxo(&mut self, utxo: RegtestUtxo) -> Result<()> {
        parse_outpoint(&utxo)?;
        let script = parse_script_hex(&utxo.script_pubkey_hex)?;
        if !is_valid_utxo_script(&script) {
            return Err(Error::InvalidUtxoScript);
        }
        self.utxos
            .retain(|existing| !(existing.txid == utxo.txid && existing.vout == utxo.vout));
        self.utxos.push(utxo);
        self.utxos.sort_by(|a, b| {
            a.txid
                .cmp(&b.txid)
                .then(a.vout.cmp(&b.vout))
                .then(a.value_sats.cmp(&b.value_sats))
        });
        Ok(())
    }

    pub fn build_withdrawal(&self, request: BitcoinWithdrawalRequest) -> Result<BuiltWithdrawal> {
        if request.amount_sats < CHANGE_DUST_SATS {
            return Err(Error::DustWithdrawal);
        }

        let recipient = Address::from_str(&request.recipient)
            .map_err(|e| Error::InvalidAddress(e.to_string()))?
            .require_network(Network::Regtest)
            .map_err(|_| Error::NonRegtestNetwork)?;
        let change_script = match request.change_script_pubkey_hex.as_ref() {
            Some(script) => Some(parse_script_hex(script)?),
            None => None,
        };
        if let Some(script) = change_script.as_ref() {
            if !is_valid_utxo_script(script) {
                return Err(Error::InvalidUtxoScript);
            }
        }
        let fee_rate = request.fee_rate_sats_per_vb.max(1).min(
            request
                .max_fee_rate_sats_per_vb
                .unwrap_or(DEFAULT_MAX_FEE_RATE_SATS_PER_VB),
        );
        let max_inputs = request.max_inputs.unwrap_or(DEFAULT_MAX_INPUTS).max(1);
        let min_confirmations = request
            .min_confirmations
            .unwrap_or(DEFAULT_MIN_UTXO_CONFIRMATIONS);
        let max_mempool_chain_length = request
            .max_mempool_chain_length
            .unwrap_or(DEFAULT_MAX_MEMPOOL_CHAIN_LENGTH);
        let spendable = self.spendable_utxos(min_confirmations, max_mempool_chain_length);

        let mut selected = Vec::new();
        let mut input_value_sats = 0;
        for utxo in spendable {
            if selected.len() >= max_inputs {
                break;
            }
            input_value_sats += utxo.value_sats;
            selected.push(utxo);
            let estimated_fee = estimate_fee_sats(
                selected.len(),
                change_script.is_some(),
                fee_rate,
                request.min_relay_fee_sats.unwrap_or(0),
            );
            if input_value_sats >= request.amount_sats + estimated_fee {
                break;
            }
        }

        if selected.is_empty() {
            return Err(Error::NoUtxos);
        }

        let mut miner_fee_sats = estimate_fee_sats(
            selected.len(),
            change_script.is_some(),
            fee_rate,
            request.min_relay_fee_sats.unwrap_or(0),
        );
        if input_value_sats < request.amount_sats + miner_fee_sats {
            return Err(Error::InsufficientFunds {
                needed_sats: request.amount_sats + miner_fee_sats,
                available_sats: input_value_sats,
            });
        }

        let mut change_value_sats = input_value_sats - request.amount_sats - miner_fee_sats;
        if change_value_sats > 0 && change_value_sats < CHANGE_DUST_SATS {
            miner_fee_sats += change_value_sats;
            change_value_sats = 0;
        }

        let mut tx = Transaction {
            version: bitcoin::transaction::Version(2),
            lock_time: LockTime::ZERO,
            input: selected
                .iter()
                .map(|utxo| TxIn {
                    previous_output: parse_outpoint(utxo)
                        .expect("UTXO was validated before selection"),
                    script_sig: ScriptBuf::new(),
                    sequence: Sequence::ENABLE_RBF_NO_LOCKTIME,
                    witness: Witness::default(),
                })
                .collect(),
            output: vec![TxOut {
                value: Amount::from_sat(request.amount_sats),
                script_pubkey: recipient.script_pubkey(),
            }],
        };

        if change_value_sats >= CHANGE_DUST_SATS {
            if let Some(script_pubkey) = change_script {
                tx.output.push(TxOut {
                    value: Amount::from_sat(change_value_sats),
                    script_pubkey,
                });
            } else {
                miner_fee_sats += change_value_sats;
                change_value_sats = 0;
            }
        }
        Ok(BuiltWithdrawal {
            withdrawal_id: request.withdrawal_id,
            unsigned_tx_hex: serialize_hex(&tx),
            input_value_sats,
            output_value_sats: request.amount_sats,
            change_value_sats,
            miner_fee_sats,
            selected_utxos: selected,
        })
    }

    pub fn build_consolidation(
        &self,
        request: BitcoinConsolidationRequest,
    ) -> Result<BuiltConsolidation> {
        let change_script = parse_script_hex(&request.change_script_pubkey_hex)?;
        if !is_valid_utxo_script(&change_script) {
            return Err(Error::InvalidUtxoScript);
        }
        let min_confirmations = request
            .min_confirmations
            .unwrap_or(DEFAULT_MIN_UTXO_CONFIRMATIONS);
        let max_mempool_chain_length = request
            .max_mempool_chain_length
            .unwrap_or(DEFAULT_MAX_MEMPOOL_CHAIN_LENGTH);
        let max_inputs = request.max_inputs.unwrap_or(DEFAULT_MAX_INPUTS).max(1);
        let min_utxos = request
            .min_utxos
            .unwrap_or(DEFAULT_CONSOLIDATION_MIN_UTXOS)
            .max(2);
        let selected = self
            .spendable_utxos(min_confirmations, max_mempool_chain_length)
            .into_iter()
            .take(max_inputs)
            .collect::<Vec<_>>();
        if selected.len() < min_utxos {
            return Err(Error::NothingToConsolidate);
        }

        let input_value_sats = selected.iter().map(|utxo| utxo.value_sats).sum::<u64>();
        let fee_rate = request.fee_rate_sats_per_vb.max(1).min(
            request
                .max_fee_rate_sats_per_vb
                .unwrap_or(DEFAULT_MAX_FEE_RATE_SATS_PER_VB),
        );
        let miner_fee_sats = estimate_fee_sats(
            selected.len(),
            false,
            fee_rate,
            request.min_relay_fee_sats.unwrap_or(0),
        );
        if input_value_sats <= miner_fee_sats + CHANGE_DUST_SATS {
            return Err(Error::InsufficientFunds {
                needed_sats: miner_fee_sats + CHANGE_DUST_SATS,
                available_sats: input_value_sats,
            });
        }
        let output_value_sats = input_value_sats - miner_fee_sats;
        let tx = Transaction {
            version: bitcoin::transaction::Version(2),
            lock_time: LockTime::ZERO,
            input: selected
                .iter()
                .map(|utxo| TxIn {
                    previous_output: parse_outpoint(utxo)
                        .expect("UTXO was validated before selection"),
                    script_sig: ScriptBuf::new(),
                    sequence: Sequence::ENABLE_RBF_NO_LOCKTIME,
                    witness: Witness::default(),
                })
                .collect(),
            output: vec![TxOut {
                value: Amount::from_sat(output_value_sats),
                script_pubkey: change_script,
            }],
        };
        Ok(BuiltConsolidation {
            consolidation_id: request.consolidation_id,
            unsigned_tx_hex: serialize_hex(&tx),
            input_value_sats,
            output_value_sats,
            miner_fee_sats,
            selected_utxos: selected,
        })
    }

    fn clone_with_excluded_outpoints(&self, excluded: &BTreeSet<String>) -> Self {
        Self {
            utxos: self
                .utxos
                .iter()
                .filter(|utxo| !excluded.contains(&outpoint_key(utxo)))
                .cloned()
                .collect(),
        }
    }

    fn spendable_utxos(
        &self,
        min_confirmations: u64,
        max_mempool_chain_length: u64,
    ) -> Vec<RegtestUtxo> {
        let mut utxos = self
            .utxos
            .iter()
            .filter(|utxo| utxo.value_sats >= CHANGE_DUST_SATS)
            .filter(|utxo| {
                if utxo.confirmations > 0 && utxo.confirmations >= min_confirmations {
                    return true;
                }
                if utxo.confirmations == 0 && utxo.is_self_transfer {
                    let chain_len = utxo.mempool_ancestor_count + utxo.mempool_descendant_count;
                    return chain_len < max_mempool_chain_length;
                }
                false
            })
            .cloned()
            .collect::<Vec<_>>();
        utxos.sort_by(|a, b| {
            b.confirmations
                .cmp(&a.confirmations)
                .then(a.txid.cmp(&b.txid))
                .then(a.vout.cmp(&b.vout))
        });
        utxos
    }
}

pub fn tx_weight_bytes(tx_hex: &str) -> Result<usize> {
    let bytes = hex::decode(tx_hex).map_err(|e| Error::InvalidScript(e.to_string()))?;
    Ok(bytes.len())
}

pub fn is_valid_utxo_script(script: &ScriptBuf) -> bool {
    is_p2wpkh(script.as_bytes())
}

fn estimate_fee_sats(
    input_count: usize,
    has_change: bool,
    fee_rate_sats_per_vb: u64,
    min_relay_fee_sats: u64,
) -> u64 {
    let change_outputs = usize::from(has_change);
    let estimated_vbytes = 10 + input_count as u64 * 68 + (1 + change_outputs) as u64 * 34;
    (estimated_vbytes * fee_rate_sats_per_vb).max(min_relay_fee_sats)
}

fn parse_outpoint(utxo: &RegtestUtxo) -> Result<OutPoint> {
    let txid = Txid::from_str(&utxo.txid).map_err(|e| Error::InvalidTxid(e.to_string()))?;
    Ok(OutPoint::new(txid, utxo.vout))
}

fn parse_script_hex(script: &str) -> Result<ScriptBuf> {
    let bytes = hex::decode(script).map_err(|e| Error::InvalidScript(e.to_string()))?;
    Ok(ScriptBuf::from_bytes(bytes))
}

fn is_p2wpkh(bytes: &[u8]) -> bool {
    bytes.len() == 22 && bytes[0] == 0x00 && bytes[1] == 0x14
}

fn outpoint_key(utxo: &RegtestUtxo) -> String {
    format!("{}:{}", utxo.txid, utxo.vout)
}

pub fn txid_for_tests(byte: u8) -> String {
    hex::encode([byte; 32])
}

pub fn script_hex(script: &ScriptBuf) -> String {
    hex::encode(script.as_bytes())
}

pub fn tx_bytes(tx_hex: &str) -> Result<Vec<u8>> {
    hex::decode(tx_hex).map_err(|e| Error::InvalidScript(e.to_string()))
}

pub fn serialized_len(tx: &Transaction) -> usize {
    serialize(tx).len()
}

fn default_confirmations() -> u64 {
    DEFAULT_MIN_UTXO_CONFIRMATIONS
}

fn transaction_from_hex(tx_hex: &str) -> Result<Transaction> {
    let bytes = tx_bytes(tx_hex)?;
    bitcoin::consensus::deserialize(&bytes).map_err(|e| Error::InvalidScript(e.to_string()))
}

fn validate_signed_tx_matches_built(
    expected_tx_hex: &str,
    signed_tx_hex: &str,
) -> Result<Transaction> {
    let expected = transaction_from_hex(expected_tx_hex)?;
    let signed = transaction_from_hex(signed_tx_hex)?;
    if expected.version != signed.version
        || expected.lock_time != signed.lock_time
        || expected.output != signed.output
        || expected.input.len() != signed.input.len()
        || expected.compute_txid() != signed.compute_txid()
    {
        return Err(Error::CheckpointMismatch);
    }
    for (expected_input, signed_input) in expected.input.iter().zip(signed.input.iter()) {
        if expected_input.previous_output != signed_input.previous_output
            || expected_input.sequence != signed_input.sequence
        {
            return Err(Error::CheckpointMismatch);
        }
    }
    Ok(signed)
}

fn validate_checkpoint_match(expected_tx_hex: &str, actual_tx_hex: &str) -> Result<()> {
    if expected_tx_hex == actual_tx_hex {
        Ok(())
    } else {
        Err(Error::CheckpointMismatch)
    }
}

fn validate_checkpoint_inputs(tx_hex: &str, utxos: &[RegtestUtxo]) -> Result<()> {
    let tx = transaction_from_hex(tx_hex)?;
    let available = utxos
        .iter()
        .map(|utxo| outpoint_key(utxo))
        .collect::<BTreeSet<_>>();
    for input in tx.input {
        let key = format!(
            "{}:{}",
            input.previous_output.txid, input.previous_output.vout
        );
        if !available.contains(&key) {
            return Err(Error::CheckpointInputsUnavailable);
        }
    }
    Ok(())
}

fn solvency_report(
    expected_sats: u64,
    utxos: &[RegtestUtxo],
    reserved_outpoints: &BTreeSet<String>,
) -> BitcoinSolvencyReport {
    let valid_utxos = utxos
        .iter()
        .filter(|utxo| {
            parse_script_hex(&utxo.script_pubkey_hex)
                .map(|script| is_valid_utxo_script(&script))
                .unwrap_or(false)
                && utxo.value_sats >= CHANGE_DUST_SATS
        })
        .collect::<Vec<_>>();
    let actual_sats = valid_utxos.iter().map(|utxo| utxo.value_sats).sum();
    let confirmed_sats = valid_utxos
        .iter()
        .filter(|utxo| utxo.confirmations > 0)
        .map(|utxo| utxo.value_sats)
        .sum::<u64>();
    let self_mempool_sats = valid_utxos
        .iter()
        .filter(|utxo| utxo.confirmations == 0 && utxo.is_self_transfer)
        .filter(|utxo| {
            utxo.mempool_ancestor_count + utxo.mempool_descendant_count
                < DEFAULT_MAX_MEMPOOL_CHAIN_LENGTH
        })
        .map(|utxo| utxo.value_sats)
        .sum::<u64>();
    let external_mempool_sats = valid_utxos
        .iter()
        .filter(|utxo| utxo.confirmations == 0 && !utxo.is_self_transfer)
        .map(|utxo| utxo.value_sats)
        .sum();
    let reserved_sats = utxos
        .iter()
        .filter(|utxo| reserved_outpoints.contains(&outpoint_key(utxo)))
        .map(|utxo| utxo.value_sats)
        .sum();
    let spendable_sats = confirmed_sats + self_mempool_sats;
    let spendable_utxo_count = valid_utxos
        .iter()
        .filter(|utxo| {
            utxo.confirmations > 0
                || (utxo.confirmations == 0
                    && utxo.is_self_transfer
                    && utxo.mempool_ancestor_count + utxo.mempool_descendant_count
                        < DEFAULT_MAX_MEMPOOL_CHAIN_LENGTH)
        })
        .count();
    BitcoinSolvencyReport {
        expected_sats,
        actual_sats,
        confirmed_sats,
        self_mempool_sats,
        external_mempool_sats,
        spendable_utxo_count,
        reserved_sats,
        solvent: spendable_sats >= expected_sats,
    }
}

fn rpc_utxos_from_value(value: &serde_json::Value) -> Result<Vec<RegtestUtxo>> {
    let utxos = value
        .as_array()
        .ok_or_else(|| Error::Rpc("listunspent result was not an array".to_string()))?;
    utxos
        .iter()
        .filter(|utxo| {
            utxo.get("spendable")
                .and_then(serde_json::Value::as_bool)
                .unwrap_or(true)
        })
        .map(|utxo| {
            Ok(RegtestUtxo {
                txid: required_str(utxo, "txid")?.to_string(),
                vout: required_u64(utxo, "vout")? as u32,
                value_sats: btc_amount_to_sats(
                    utxo.get("amount")
                        .ok_or_else(|| Error::Rpc("listunspent UTXO missing amount".to_string()))?,
                )?,
                script_pubkey_hex: required_str(utxo, "scriptPubKey")?.to_string(),
                confirmations: utxo
                    .get("confirmations")
                    .and_then(serde_json::Value::as_u64)
                    .unwrap_or_default(),
                is_self_transfer: false,
                mempool_ancestor_count: utxo
                    .get("ancestorcount")
                    .and_then(serde_json::Value::as_u64)
                    .unwrap_or_default(),
                mempool_descendant_count: utxo
                    .get("descendantcount")
                    .and_then(serde_json::Value::as_u64)
                    .unwrap_or_default(),
            })
        })
        .collect()
}

fn required_str<'a>(value: &'a serde_json::Value, key: &str) -> Result<&'a str> {
    value
        .get(key)
        .and_then(serde_json::Value::as_str)
        .ok_or_else(|| Error::Rpc(format!("RPC object missing string field {key}")))
}

fn required_u64(value: &serde_json::Value, key: &str) -> Result<u64> {
    value
        .get(key)
        .and_then(serde_json::Value::as_u64)
        .ok_or_else(|| Error::Rpc(format!("RPC object missing integer field {key}")))
}

fn btc_amount_to_sats(value: &serde_json::Value) -> Result<u64> {
    if let Some(number) = value.as_f64() {
        return Ok((number * 100_000_000.0).round() as u64);
    }
    if let Some(text) = value.as_str() {
        let number = text
            .parse::<f64>()
            .map_err(|e| Error::Rpc(format!("invalid BTC amount: {e}")))?;
        return Ok((number * 100_000_000.0).round() as u64);
    }
    Err(Error::Rpc("invalid BTC amount value".to_string()))
}

fn rpc_http_post(url: &str, user: &str, password: &str, body: &str) -> Result<String> {
    let target = RpcUrl::parse(url)?;
    let mut stream = TcpStream::connect((target.host.as_str(), target.port))
        .map_err(|e| Error::Rpc(format!("connect failed: {e}")))?;
    let auth = basic_auth(user, password);
    let request = format!(
        "POST {} HTTP/1.1\r\nHost: {}\r\nAuthorization: Basic {}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        target.path,
        target.host,
        auth,
        body.len(),
        body
    );
    stream
        .write_all(request.as_bytes())
        .map_err(|e| Error::Rpc(format!("write failed: {e}")))?;
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(|e| Error::Rpc(format!("read failed: {e}")))?;
    let (headers, body) = response
        .split_once("\r\n\r\n")
        .ok_or_else(|| Error::Rpc("malformed HTTP response".to_string()))?;
    if !headers.starts_with("HTTP/1.1 200") && !headers.starts_with("HTTP/1.0 200") {
        return Err(Error::Rpc(
            headers.lines().next().unwrap_or(headers).to_string(),
        ));
    }
    Ok(body.to_string())
}

#[derive(Debug)]
struct RpcUrl {
    host: String,
    port: u16,
    path: String,
}

impl RpcUrl {
    fn parse(url: &str) -> Result<Self> {
        let url = url
            .strip_prefix("http://")
            .ok_or_else(|| Error::Rpc("only http:// Bitcoin RPC URLs are supported".to_string()))?;
        let (authority, path) = match url.split_once('/') {
            Some((authority, path)) => (authority, format!("/{path}")),
            None => (url, "/".to_string()),
        };
        let (host, port) = match authority.rsplit_once(':') {
            Some((host, port)) => (
                host.to_string(),
                port.parse()
                    .map_err(|e| Error::Rpc(format!("invalid RPC port: {e}")))?,
            ),
            None => (authority.to_string(), 18443),
        };
        Ok(Self { host, port, path })
    }
}

fn basic_auth(user: &str, password: &str) -> String {
    base64_encode(format!("{user}:{password}").as_bytes())
}

fn base64_encode(bytes: &[u8]) -> String {
    const TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::new();
    for chunk in bytes.chunks(3) {
        let b0 = chunk[0];
        let b1 = *chunk.get(1).unwrap_or(&0);
        let b2 = *chunk.get(2).unwrap_or(&0);
        out.push(TABLE[(b0 >> 2) as usize] as char);
        out.push(TABLE[(((b0 & 0b0000_0011) << 4) | (b1 >> 4)) as usize] as char);
        if chunk.len() > 1 {
            out.push(TABLE[(((b1 & 0b0000_1111) << 2) | (b2 >> 6)) as usize] as char);
        } else {
            out.push('=');
        }
        if chunk.len() > 2 {
            out.push(TABLE[(b2 & 0b0011_1111) as usize] as char);
        } else {
            out.push('=');
        }
    }
    out
}
