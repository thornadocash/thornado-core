use tendermint_abci::Application;
use tendermint_proto::v0_38::abci::{RequestCheckTx, RequestFinalizeBlock};
use thornado_abci::{decode_tx, encode_tx, ThornadoAbciApp};
use thornado_core::{
    derive_split_receipt, execute_command, mine_deposit_pow, state_hash, AppState, Command,
    MockCustodySigner, MockProofVerifier,
};

fn seeded_state() -> AppState {
    let mut state = AppState::default();
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: mine_deposit_pow("abci-bootstrap"),
            user_pubkey: "abci-client-pubkey".to_string(),
        },
        &MockProofVerifier,
        &MockCustodySigner,
    )
    .unwrap();
    state
}

fn deposit_block(prefix: &str) -> Vec<Vec<u8>> {
    let amount_sats = 100_000_000;
    let receipt = derive_split_receipt("dep-2", amount_sats, prefix).unwrap();
    [
        Command::RequestDepositAddress {
            pow_token: mine_deposit_pow(prefix),
            user_pubkey: "abci-client-pubkey".to_string(),
        },
        Command::ObserveDeposit {
            intent_id: "dep-2".to_string(),
            txid: format!("{prefix}-txid"),
            amount_sats,
        },
        Command::ConfirmDeposit {
            intent_id: "dep-2".to_string(),
        },
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-2".to_string(),
            note_commitments: receipt.commitments(),
        },
    ]
    .into_iter()
    .map(|command| encode_tx(command).unwrap())
    .collect()
}

#[test]
fn tx_encoding_round_trips() {
    let command = Command::MarkNodeOffline {
        node_id: "node-3".to_string(),
    };

    let decoded = decode_tx(&encode_tx(command.clone()).unwrap()).unwrap();

    assert_eq!(decoded.command, command);
}

#[test]
fn check_tx_does_not_mutate_state() {
    let app = ThornadoAbciApp::new(seeded_state());
    let before = app.status();
    let tx = encode_tx(Command::RequestDepositAddress {
        pow_token: mine_deposit_pow("abci-check"),
        user_pubkey: "abci-client-pubkey".to_string(),
    })
    .unwrap();

    app.check_tx_bytes(&tx).unwrap();

    assert_eq!(app.status(), before);
}

#[test]
fn finalize_block_applies_transactions_and_updates_app_hash() {
    let app = ThornadoAbciApp::new(seeded_state());
    let before = app.status();

    let results = app.finalize_block_bytes(&deposit_block("abci-finalize"));

    assert!(results.iter().all(Result::is_ok));
    assert_eq!(app.status().height, before.height + 1);
    assert_ne!(app.status().app_hash, before.app_hash);
}

#[test]
fn same_block_from_same_genesis_produces_same_app_hash() {
    let genesis = seeded_state();
    let first = ThornadoAbciApp::new(genesis.clone());
    let second = ThornadoAbciApp::new(genesis);
    let txs = deposit_block("abci-shared");

    assert!(first.finalize_block_bytes(&txs).iter().all(Result::is_ok));
    assert!(second.finalize_block_bytes(&txs).iter().all(Result::is_ok));

    assert_eq!(first.status(), second.status());
}

#[test]
fn invalid_tx_is_rejected_by_abci_check_tx() {
    let app = ThornadoAbciApp::new(seeded_state());

    let response = app.check_tx(RequestCheckTx {
        tx: b"not-json".to_vec().into(),
        ..Default::default()
    });

    assert_eq!(response.code, 1);
}

#[test]
fn request_deposit_requires_genesis_custody_keyset() {
    let app = ThornadoAbciApp::new(AppState::default());
    let tx = encode_tx(Command::RequestDepositAddress {
        pow_token: mine_deposit_pow("abci-no-keyset"),
        user_pubkey: "abci-client-pubkey".to_string(),
    })
    .unwrap();

    let results = app.finalize_block(RequestFinalizeBlock {
        txs: vec![tx.into()],
        ..Default::default()
    });

    assert_eq!(results.tx_results[0].code, 1);
    assert_eq!(state_hash(&app.current_state()), app.status().app_hash);
}

#[test]
fn churn_key_generation_is_rejected_in_abci() {
    let app = ThornadoAbciApp::new(seeded_state());
    let tx = encode_tx(Command::StartChurnEpoch).unwrap();

    let result = app.finalize_block_bytes(&[tx]);

    assert!(result[0].is_err());
}
