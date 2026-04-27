use bitcoin::absolute::LockTime;
use bitcoin::consensus::encode::{serialize, serialize_hex};
use bitcoin::{
    Address, Amount, Network, OutPoint, ScriptBuf, Sequence, Transaction, TxIn, TxOut, Txid,
    Witness,
};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};
use std::str::FromStr;

pub type Result<T> = std::result::Result<T, Error>;

pub const DEFAULT_FEE_RATE_SATS_PER_VB: u64 = 2;
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
    #[error("bitcoin withdrawal already exists")]
    WithdrawalAlreadyBuilt,
    #[error("bitcoin withdrawal not found")]
    WithdrawalNotFound,
    #[error("UTXO is already reserved")]
    UtxoReserved,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RegtestUtxo {
    pub txid: String,
    pub vout: u32,
    pub value_sats: u64,
    pub script_pubkey_hex: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinWithdrawalRequest {
    pub withdrawal_id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_rate_sats_per_vb: u64,
    pub change_script_pubkey_hex: Option<String>,
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

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct DevBitcoinBackend {
    vault: RegtestVault,
    built_withdrawals: BTreeMap<String, BitcoinWithdrawalRecord>,
    reserved_outpoints: BTreeSet<String>,
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
}

impl RegtestVault {
    pub fn import_utxo(&mut self, utxo: RegtestUtxo) -> Result<()> {
        parse_outpoint(&utxo)?;
        parse_script_hex(&utxo.script_pubkey_hex)?;
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
        let fee_rate = request.fee_rate_sats_per_vb.max(1);

        let mut selected = Vec::new();
        let mut input_value_sats = 0;
        for utxo in &self.utxos {
            selected.push(utxo.clone());
            input_value_sats += utxo.value_sats;
            let estimated_fee =
                estimate_fee_sats(selected.len(), change_script.is_some(), fee_rate);
            if input_value_sats >= request.amount_sats + estimated_fee {
                break;
            }
        }

        if selected.is_empty() {
            return Err(Error::NoUtxos);
        }

        let mut miner_fee_sats =
            estimate_fee_sats(selected.len(), change_script.is_some(), fee_rate);
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
}

pub fn tx_weight_bytes(tx_hex: &str) -> Result<usize> {
    let bytes = hex::decode(tx_hex).map_err(|e| Error::InvalidScript(e.to_string()))?;
    Ok(bytes.len())
}

fn estimate_fee_sats(input_count: usize, has_change: bool, fee_rate_sats_per_vb: u64) -> u64 {
    let output_count = if has_change { 2 } else { 1 };
    let estimated_vbytes = 10 + input_count as u64 * 68 + output_count as u64 * 34;
    estimated_vbytes * fee_rate_sats_per_vb
}

fn parse_outpoint(utxo: &RegtestUtxo) -> Result<OutPoint> {
    let txid = Txid::from_str(&utxo.txid).map_err(|e| Error::InvalidTxid(e.to_string()))?;
    Ok(OutPoint::new(txid, utxo.vout))
}

fn parse_script_hex(script: &str) -> Result<ScriptBuf> {
    let bytes = hex::decode(script).map_err(|e| Error::InvalidScript(e.to_string()))?;
    Ok(ScriptBuf::from_bytes(bytes))
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
