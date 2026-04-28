use serde::{Deserialize, Serialize};
use std::sync::{Arc, Mutex};
use tendermint_abci::Application;
use tendermint_proto::v0_38::abci::{
    ExecTxResult, RequestCheckTx, RequestFinalizeBlock, RequestInfo, RequestQuery, ResponseCheckTx,
    ResponseCommit, ResponseFinalizeBlock, ResponseInfo, ResponseQuery,
};
use thornado_core::{
    execute_command, start_churn_epoch_without_keygen, state_hash, validate_keyset_commit,
    AppState, AuthorizedWithdrawal, Command, Error as CoreError, MockCustodySigner,
    MockProofVerifier, PendingWithdrawal, WithdrawalProof, ZkProofVerifier,
};

pub type Result<T> = std::result::Result<T, Error>;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("transaction decode error: {0}")]
    Decode(String),
    #[error("transaction execution error: {0}")]
    Execution(String),
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ThornadoTx {
    pub command: Command,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AbciStatus {
    pub height: i64,
    pub app_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AbciBifrostKeygenWork {
    pub pending: bool,
    pub epoch: u64,
    pub participants: Vec<String>,
    pub threshold: u16,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AbciBifrostWithdrawalsWork {
    pub pending: Vec<PendingWithdrawal>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AbciBifrostBitcoinWork {
    #[serde(default)]
    pub outbounds: Vec<thornado_core::BitcoinOutbound>,
    #[serde(default)]
    pub deposit_sweeps: Vec<AbciDepositSweepWork>,
    #[serde(default)]
    pub authorized: Vec<AuthorizedWithdrawal>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AbciDepositSweepWork {
    pub deposit_id: String,
    pub txid: String,
    pub custody_epoch: u64,
    pub deposit_key_tweak: String,
    pub vault_signers: Vec<String>,
    pub vault_threshold: u16,
}

#[derive(Clone)]
pub struct ThornadoAbciApp {
    state: Arc<Mutex<AppState>>,
    height: Arc<Mutex<i64>>,
}

impl ThornadoAbciApp {
    pub fn new(genesis_state: AppState) -> Self {
        Self {
            state: Arc::new(Mutex::new(genesis_state)),
            height: Arc::new(Mutex::new(0)),
        }
    }

    pub fn status(&self) -> AbciStatus {
        let state = self.state.lock().expect("ABCI state mutex poisoned");
        let height = *self.height.lock().expect("ABCI height mutex poisoned");
        AbciStatus {
            height,
            app_hash: state_hash(&state),
        }
    }

    pub fn current_state(&self) -> AppState {
        self.state
            .lock()
            .expect("ABCI state mutex poisoned")
            .clone()
    }

    pub fn check_tx_bytes(&self, tx: &[u8]) -> Result<()> {
        let tx = decode_tx(tx)?;
        let mut state = self.current_state();
        apply_command(&mut state, tx.command)?;
        Ok(())
    }

    pub fn finalize_block_bytes(&self, txs: &[Vec<u8>]) -> Vec<Result<()>> {
        let mut state = self.state.lock().expect("ABCI state mutex poisoned");
        let mut results = Vec::with_capacity(txs.len());
        for tx in txs {
            let result = decode_tx(tx).and_then(|tx| apply_command(&mut state, tx.command));
            results.push(result);
        }
        *self.height.lock().expect("ABCI height mutex poisoned") += 1;
        results
    }

    pub fn commit_hash(&self) -> Vec<u8> {
        hex::decode(self.status().app_hash).expect("state hash is hex")
    }
}

impl Application for ThornadoAbciApp {
    fn info(&self, _request: RequestInfo) -> ResponseInfo {
        let status = self.status();
        ResponseInfo {
            data: "thornado-abci".to_string(),
            version: env!("CARGO_PKG_VERSION").to_string(),
            app_version: 1,
            last_block_height: status.height,
            last_block_app_hash: hex::decode(status.app_hash).unwrap_or_default().into(),
        }
    }

    fn query(&self, request: RequestQuery) -> ResponseQuery {
        match request.path.as_str() {
            "/state/hash" => ResponseQuery {
                code: 0,
                value: self.status().app_hash.into_bytes().into(),
                log: "ok".to_string(),
                ..Default::default()
            },
            "/state" => response_json(&self.current_state()),
            "/bifrost/work/keygen" => {
                let state = self.current_state();
                response_json(&keygen_work(&state))
            }
            "/bifrost/work/withdrawals" => {
                let state = self.current_state();
                response_json(&AbciBifrostWithdrawalsWork {
                    pending: state.withdrawals.pending.values().cloned().collect(),
                })
            }
            "/bifrost/work/bitcoin" => {
                let state = self.current_state();
                response_json(&AbciBifrostBitcoinWork {
                    outbounds: state
                        .withdrawals
                        .bitcoin_outbounds
                        .values()
                        .filter(|outbound| outbound.published_txid.is_none())
                        .cloned()
                        .collect(),
                    deposit_sweeps: state
                        .deposits
                        .intents
                        .values()
                        .filter(|intent| intent.confirmed && intent.swept_txid.is_none())
                        .filter_map(|intent| {
                            Some(AbciDepositSweepWork {
                                deposit_id: intent.id.clone(),
                                txid: intent.txid.clone()?,
                                custody_epoch: intent.custody_epoch,
                                deposit_key_tweak: intent.deposit_key_tweak.clone(),
                                vault_signers: intent.vault_signers.clone(),
                                vault_threshold: intent.vault_threshold,
                            })
                        })
                        .collect(),
                    authorized: state
                        .withdrawals
                        .authorized
                        .values()
                        .filter(|withdrawal| {
                            !state
                                .withdrawals
                                .bitcoin_broadcasts
                                .contains_key(&withdrawal.id)
                        })
                        .cloned()
                        .collect(),
                })
            }
            _ => ResponseQuery {
                code: 1,
                log: format!("unknown query path {}", request.path),
                ..Default::default()
            },
        }
    }

    fn check_tx(&self, request: RequestCheckTx) -> ResponseCheckTx {
        match self.check_tx_bytes(&request.tx) {
            Ok(()) => ResponseCheckTx {
                code: 0,
                log: "accepted".to_string(),
                ..Default::default()
            },
            Err(error) => ResponseCheckTx {
                code: 1,
                log: error.to_string(),
                ..Default::default()
            },
        }
    }

    fn finalize_block(&self, request: RequestFinalizeBlock) -> ResponseFinalizeBlock {
        let txs = request.txs.iter().map(|tx| tx.to_vec()).collect::<Vec<_>>();
        let results = self.finalize_block_bytes(&txs);
        ResponseFinalizeBlock {
            tx_results: results
                .into_iter()
                .map(|result| match result {
                    Ok(()) => ExecTxResult {
                        code: 0,
                        log: "ok".to_string(),
                        ..Default::default()
                    },
                    Err(error) => ExecTxResult {
                        code: 1,
                        log: error.to_string(),
                        ..Default::default()
                    },
                })
                .collect(),
            app_hash: self.commit_hash().into(),
            ..Default::default()
        }
    }

    fn commit(&self) -> ResponseCommit {
        ResponseCommit { retain_height: 0 }
    }
}

fn response_json<T: Serialize>(value: &T) -> ResponseQuery {
    match serde_json::to_vec(value) {
        Ok(value) => ResponseQuery {
            code: 0,
            value: value.into(),
            log: "ok".to_string(),
            ..Default::default()
        },
        Err(error) => ResponseQuery {
            code: 1,
            log: error.to_string(),
            ..Default::default()
        },
    }
}

fn keygen_work(state: &AppState) -> AbciBifrostKeygenWork {
    let epoch = state.churn.epoch;
    let participants = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
    let pending = participants.len() >= 2 && !state.custody.keysets.contains_key(&epoch);
    let threshold = if pending {
        thornado_core::frost_threshold_for_committee(participants.len() as u16)
    } else {
        0
    };
    AbciBifrostKeygenWork {
        pending,
        epoch,
        participants,
        threshold,
    }
}

pub fn encode_tx(command: Command) -> Result<Vec<u8>> {
    serde_json::to_vec(&ThornadoTx { command }).map_err(|error| Error::Decode(error.to_string()))
}

pub fn decode_tx(bytes: &[u8]) -> Result<ThornadoTx> {
    serde_json::from_slice(bytes).map_err(|error| Error::Decode(error.to_string()))
}

fn apply_command(state: &mut AppState, command: Command) -> Result<()> {
    guard_consensus_determinism(state, &command)?;
    if let Command::StartChurnEpoch = command {
        apply_consensus_churn(state)?;
        return Ok(());
    }
    let signer = MockCustodySigner;
    execute_command_secure(state, command, &signer)
        .map(|_| ())
        .map_err(|error| Error::Execution(error.to_string()))
}

fn guard_consensus_determinism(state: &AppState, command: &Command) -> Result<()> {
    match command {
        Command::RequestDepositAddress { .. }
            if !state
                .custody
                .keysets
                .contains_key(&state.custody.active_epoch) =>
        {
            Err(Error::Execution(
                "missing genesis custody keyset; refusing nondeterministic key generation"
                    .to_string(),
            ))
        }
        Command::CommitCustodyKeyset { epoch, keyset } => {
            validate_keyset_commit(state, *epoch, keyset)
                .map_err(|error| Error::Execution(error.to_string()))
        }
        _ => Ok(()),
    }
}

fn apply_consensus_churn(state: &mut AppState) -> Result<()> {
    start_churn_epoch_without_keygen(state)
        .map(|_| ())
        .map_err(|error| Error::Execution(error.to_string()))
}

fn execute_command_secure(
    state: &mut AppState,
    command: Command,
    signer: &MockCustodySigner,
) -> thornado_core::Result<Vec<thornado_core::Event>> {
    match &command {
        Command::WithdrawNote { proof, .. } | Command::RequestWithdrawal { proof, .. } => {
            reject_secret_bearing_proof(proof)?;
            execute_command(state, command, &ZkProofVerifier, signer)
        }
        _ => execute_command(state, command, &MockProofVerifier, signer),
    }
}

fn reject_secret_bearing_proof(proof: &WithdrawalProof) -> thornado_core::Result<()> {
    if proof.nullifier.is_empty() && proof.secret.is_empty() && proof.commitment.is_empty() {
        Ok(())
    } else {
        Err(CoreError::InvalidProof)
    }
}
