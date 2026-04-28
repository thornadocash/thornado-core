use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, CompressedPublicKey, Network};
use std::io::{Read, Write};
use std::net::TcpListener;
use std::thread;
use thornado_bitcoin::{
    script_hex, tx_bytes, txid_for_tests, BitcoinBackend, BitcoinConsolidationRequest,
    BitcoinRpcConfig, BitcoinWithdrawalRequest, DevBitcoinBackend, Error, RegtestUtxo,
    RegtestVault, RpcBitcoinBackend,
};

#[test]
fn builds_unsigned_regtest_withdrawal_from_imported_utxo() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(regtest_utxo(1, 150_000, change_script_hex()))
        .unwrap();

    let built = vault
        .build_withdrawal(withdrawal_request("wd-1", Network::Regtest))
        .unwrap();

    assert_eq!(built.withdrawal_id, "wd-1");
    assert_eq!(built.output_value_sats, 100_000);
    assert_eq!(built.selected_utxos.len(), 1);
    assert!(built.miner_fee_sats > 0);
    assert!(built.change_value_sats >= 546);
    assert!(!tx_bytes(&built.unsigned_tx_hex).unwrap().is_empty());
}

#[test]
fn rejects_non_regtest_recipient() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(regtest_utxo(2, 150_000, change_script_hex()))
        .unwrap();

    let err = vault
        .build_withdrawal(withdrawal_request("wd-1", Network::Bitcoin))
        .unwrap_err();

    assert_eq!(err, Error::NonRegtestNetwork);
}

#[test]
fn rejects_insufficient_funds() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(regtest_utxo(3, 20_000, change_script_hex()))
        .unwrap();

    let err = vault
        .build_withdrawal(withdrawal_request("wd-1", Network::Regtest))
        .unwrap_err();

    assert!(matches!(err, Error::InsufficientFunds { .. }));
}

#[test]
fn dev_backend_records_lifecycle_and_reserves_utxos() {
    let mut backend = DevBitcoinBackend::new();
    backend
        .import_dev_utxo(regtest_utxo(4, 150_000, change_script_hex()))
        .unwrap();

    let built = backend
        .build_withdrawal(withdrawal_request("wd-1", Network::Regtest))
        .unwrap();
    assert_eq!(built.withdrawal_id, "wd-1");

    let err = backend
        .build_withdrawal(withdrawal_request("wd-2", Network::Regtest))
        .unwrap_err();
    assert!(matches!(
        err,
        Error::NoUtxos | Error::InsufficientFunds { .. }
    ));

    let txid = txid_for_tests(5);
    let record = backend.mark_broadcast("wd-1", txid.clone()).unwrap();
    assert_eq!(record.broadcast_txid.as_deref(), Some(txid.as_str()));
    let record = backend.mark_confirmed("wd-1", 101).unwrap();
    assert_eq!(record.confirmed_height, Some(101));
    assert_eq!(backend.get_withdrawal("wd-1").unwrap(), record);
}

#[test]
fn rpc_backend_reads_regtest_utxos_and_broadcasts() {
    let expected_txid = txid_for_tests(10);
    let rpc = spawn_rpc_server(vec![
        r#"{"result":{"chain":"regtest"},"error":null,"id":"thornado"}"#.to_string(),
        format!(
            r#"{{"result":[{{"txid":"{}","vout":0,"amount":0.002,"scriptPubKey":"{}","spendable":true,"confirmations":1}}],"error":null,"id":"thornado"}}"#,
            txid_for_tests(9),
            change_script_hex()
        ),
        format!(
            r#"{{"result":[{{"txid":"{}","vout":0,"amount":0.002,"scriptPubKey":"{}","spendable":true,"confirmations":1}}],"error":null,"id":"thornado"}}"#,
            txid_for_tests(9),
            change_script_hex()
        ),
        format!(
            r#"{{"result":[{{"txid":"{}","vout":0,"amount":0.002,"scriptPubKey":"{}","spendable":true,"confirmations":1}}],"error":null,"id":"thornado"}}"#,
            txid_for_tests(9),
            change_script_hex()
        ),
        format!(
            r#"{{"result":"{}","error":null,"id":"thornado"}}"#,
            expected_txid
        ),
    ]);
    let mut backend = RpcBitcoinBackend::new(BitcoinRpcConfig {
        url: rpc,
        user: "user".to_string(),
        password: "password".to_string(),
    })
    .unwrap();

    assert_eq!(backend.list_utxos().len(), 1);
    let built = backend
        .build_withdrawal(withdrawal_request("wd-rpc", Network::Regtest))
        .unwrap();
    assert_eq!(built.selected_utxos.len(), 1);

    let record = backend
        .broadcast_withdrawal("wd-rpc", built.unsigned_tx_hex)
        .unwrap();
    assert_eq!(
        record.broadcast_txid.as_deref(),
        Some(expected_txid.as_str())
    );
}

#[test]
fn clamps_fee_rate_and_honors_min_relay_fee() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(regtest_utxo(11, 200_000, change_script_hex()))
        .unwrap();

    let built = vault
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-fee".to_string(),
            recipient: recipient(Network::Regtest),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 10_000,
            change_script_pubkey_hex: Some(change_script_hex()),
            max_fee_rate_sats_per_vb: Some(2),
            min_relay_fee_sats: Some(10_000),
            max_inputs: None,
            min_confirmations: None,
            max_mempool_chain_length: None,
        })
        .unwrap();

    assert_eq!(built.miner_fee_sats, 10_000);
}

#[test]
fn rejects_unsupported_bare_multisig_utxo_script() {
    let mut vault = RegtestVault::default();
    let err = vault
        .import_utxo(regtest_utxo(
            12,
            100_000,
            "51210281feb90c058c3436f8bc361930ae99fcfb530a699cdad141d7244bfcad521a1f51ae"
                .to_string(),
        ))
        .unwrap_err();

    assert_eq!(err, Error::InvalidUtxoScript);
}

#[test]
fn rejects_non_p2wpkh_spend_scripts() {
    let mut vault = RegtestVault::default();
    let p2tr_script = format!("5120{}", hex::encode([1_u8; 32]));
    let err = vault
        .import_utxo(regtest_utxo(13, 100_000, p2tr_script))
        .unwrap_err();

    assert_eq!(err, Error::InvalidUtxoScript);
}

#[test]
fn selects_confirmed_utxos_before_self_mempool_and_skips_external_mempool() {
    let mut vault = RegtestVault::default();
    let mut external_mempool = regtest_utxo(20, 150_000, change_script_hex());
    external_mempool.confirmations = 0;
    let mut self_mempool = regtest_utxo(21, 150_000, change_script_hex());
    self_mempool.confirmations = 0;
    self_mempool.is_self_transfer = true;
    let confirmed = regtest_utxo(22, 60_000, change_script_hex());
    vault.import_utxo(external_mempool).unwrap();
    vault.import_utxo(self_mempool).unwrap();
    vault.import_utxo(confirmed).unwrap();

    let mut request = withdrawal_request("wd-mempool", Network::Regtest);
    request.amount_sats = 100_000;
    let built = vault.build_withdrawal(request).unwrap();

    assert_eq!(built.selected_utxos.len(), 2);
    assert_eq!(built.selected_utxos[0].txid, txid_for_tests(22));
    assert_eq!(built.selected_utxos[1].txid, txid_for_tests(21));
}

#[test]
fn dev_backend_does_not_select_external_zero_conf_utxos_by_default() {
    let mut backend = DevBitcoinBackend::new();
    let mut external_mempool = regtest_utxo(19, 150_000, change_script_hex());
    external_mempool.confirmations = 0;
    backend.import_dev_utxo(external_mempool).unwrap();

    let err = backend
        .build_withdrawal(withdrawal_request("wd-zero-conf", Network::Regtest))
        .unwrap_err();

    assert_eq!(err, Error::NoUtxos);
}

#[test]
fn skips_self_mempool_utxo_at_chain_limit() {
    let mut vault = RegtestVault::default();
    let mut chained = regtest_utxo(23, 150_000, change_script_hex());
    chained.confirmations = 0;
    chained.is_self_transfer = true;
    chained.mempool_ancestor_count = 20;
    chained.mempool_descendant_count = 5;
    vault.import_utxo(chained).unwrap();

    let err = vault
        .build_withdrawal(withdrawal_request("wd-chain-limit", Network::Regtest))
        .unwrap_err();

    assert_eq!(err, Error::NoUtxos);
}

#[test]
fn reports_solvency_with_reserved_and_mempool_buckets() {
    let mut backend = DevBitcoinBackend::new();
    backend
        .import_dev_utxo(regtest_utxo(24, 150_000, change_script_hex()))
        .unwrap();
    let mut self_mempool = regtest_utxo(25, 50_000, change_script_hex());
    self_mempool.confirmations = 0;
    self_mempool.is_self_transfer = true;
    backend.import_dev_utxo(self_mempool).unwrap();
    let mut external_mempool = regtest_utxo(26, 25_000, change_script_hex());
    external_mempool.confirmations = 0;
    backend.import_dev_utxo(external_mempool).unwrap();

    backend
        .build_withdrawal(withdrawal_request("wd-solvency", Network::Regtest))
        .unwrap();
    let report = backend.report_solvency(200_000).unwrap();

    assert!(report.solvent);
    assert_eq!(report.actual_sats, 225_000);
    assert_eq!(report.confirmed_sats, 150_000);
    assert_eq!(report.self_mempool_sats, 50_000);
    assert_eq!(report.external_mempool_sats, 25_000);
    assert_eq!(report.reserved_sats, 150_000);
    assert_eq!(report.spendable_utxo_count, 2);

    let report = backend.report_solvency(225_000).unwrap();
    assert!(!report.solvent);
}

#[test]
fn builds_consolidation_when_enough_utxos_exist() {
    let mut backend = DevBitcoinBackend::new();
    for byte in 30..34 {
        backend
            .import_dev_utxo(regtest_utxo(byte, 50_000, change_script_hex()))
            .unwrap();
    }

    let built = backend
        .build_consolidation(BitcoinConsolidationRequest {
            consolidation_id: "consolidate-1".to_string(),
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: change_script_hex(),
            min_utxos: Some(4),
            max_inputs: None,
            min_confirmations: None,
            max_mempool_chain_length: None,
            max_fee_rate_sats_per_vb: None,
            min_relay_fee_sats: None,
        })
        .unwrap();

    assert_eq!(built.selected_utxos.len(), 4);
    assert_eq!(built.input_value_sats, 200_000);
    assert!(built.output_value_sats < built.input_value_sats);
    assert!(!tx_bytes(&built.unsigned_tx_hex).unwrap().is_empty());
    let record = backend.get_consolidation("consolidate-1").unwrap();
    assert_eq!(record.built, built);

    let record = backend
        .broadcast_consolidation("consolidate-1", built.unsigned_tx_hex)
        .unwrap();
    assert!(record.broadcast_txid.is_some());
}

#[test]
fn rejects_duplicate_consolidation_ids() {
    let mut backend = DevBitcoinBackend::new();
    for byte in 34..38 {
        backend
            .import_dev_utxo(regtest_utxo(byte, 50_000, change_script_hex()))
            .unwrap();
    }

    let request = BitcoinConsolidationRequest {
        consolidation_id: "consolidate-dup".to_string(),
        fee_rate_sats_per_vb: 2,
        change_script_pubkey_hex: change_script_hex(),
        min_utxos: Some(2),
        max_inputs: Some(2),
        min_confirmations: None,
        max_mempool_chain_length: None,
        max_fee_rate_sats_per_vb: None,
        min_relay_fee_sats: None,
    };
    backend.build_consolidation(request.clone()).unwrap();

    let err = backend.build_consolidation(request).unwrap_err();
    assert_eq!(err, Error::ConsolidationAlreadyBuilt);
}

#[test]
fn validates_signing_checkpoint_and_rejects_spent_inputs() {
    let mut backend = DevBitcoinBackend::new();
    backend
        .import_dev_utxo(regtest_utxo(40, 150_000, change_script_hex()))
        .unwrap();
    let built = backend
        .build_withdrawal(withdrawal_request("wd-checkpoint", Network::Regtest))
        .unwrap();
    let validation = backend
        .validate_signing_checkpoint("wd-checkpoint", built.unsigned_tx_hex.clone())
        .unwrap();
    assert!(validation.valid);
    assert_eq!(validation.input_count, 1);

    let err = backend
        .validate_signing_checkpoint("wd-checkpoint", signed_tx_hex_for_tests())
        .unwrap_err();
    assert_eq!(err, Error::CheckpointMismatch);
}

#[test]
fn broadcast_rejects_signed_payload_for_different_transaction() {
    let mut backend = DevBitcoinBackend::new();
    backend
        .import_dev_utxo(regtest_utxo(39, 150_000, change_script_hex()))
        .unwrap();
    backend
        .build_withdrawal(withdrawal_request("wd-broadcast-check", Network::Regtest))
        .unwrap();

    let err = backend
        .broadcast_withdrawal("wd-broadcast-check", signed_tx_hex_for_tests())
        .unwrap_err();

    assert_eq!(err, Error::CheckpointMismatch);
}

#[test]
fn rpc_checkpoint_validation_rejects_spent_inputs() {
    let rpc = spawn_rpc_server(vec![
        r#"{"result":{"chain":"regtest"},"error":null,"id":"thornado"}"#.to_string(),
        format!(
            r#"{{"result":[{{"txid":"{}","vout":0,"amount":0.002,"scriptPubKey":"{}","spendable":true,"confirmations":1}}],"error":null,"id":"thornado"}}"#,
            txid_for_tests(41),
            change_script_hex()
        ),
        r#"{"result":[],"error":null,"id":"thornado"}"#.to_string(),
    ]);
    let mut backend = RpcBitcoinBackend::new(BitcoinRpcConfig {
        url: rpc,
        user: "user".to_string(),
        password: "password".to_string(),
    })
    .unwrap();
    let built = backend
        .build_withdrawal(withdrawal_request("wd-spent", Network::Regtest))
        .unwrap();

    let err = backend
        .validate_signing_checkpoint("wd-spent", built.unsigned_tx_hex)
        .unwrap_err();
    assert_eq!(err, Error::CheckpointInputsUnavailable);
}

fn change_script_hex() -> String {
    script_hex(&address(Network::Regtest).script_pubkey())
}

fn recipient(network: Network) -> String {
    address(network).to_string()
}

fn regtest_utxo(byte: u8, value_sats: u64, script_pubkey_hex: String) -> RegtestUtxo {
    RegtestUtxo {
        txid: txid_for_tests(byte),
        vout: 0,
        value_sats,
        script_pubkey_hex,
        confirmations: 1,
        is_self_transfer: false,
        mempool_ancestor_count: 0,
        mempool_descendant_count: 0,
    }
}

fn withdrawal_request(id: &str, network: Network) -> BitcoinWithdrawalRequest {
    BitcoinWithdrawalRequest {
        withdrawal_id: id.to_string(),
        recipient: recipient(network),
        amount_sats: 100_000,
        fee_rate_sats_per_vb: 2,
        change_script_pubkey_hex: Some(change_script_hex()),
        max_fee_rate_sats_per_vb: None,
        min_relay_fee_sats: None,
        max_inputs: None,
        min_confirmations: None,
        max_mempool_chain_length: None,
    }
}

fn address(network: Network) -> Address {
    let secp = Secp256k1::new();
    let secret = SecretKey::from_slice(&[1_u8; 32]).unwrap();
    let public_key = CompressedPublicKey(SecpPublicKey::from_secret_key(&secp, &secret));
    Address::p2wpkh(&public_key, network)
}

fn spawn_rpc_server(responses: Vec<String>) -> String {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    thread::spawn(move || {
        for response in responses {
            let (mut stream, _) = listener.accept().unwrap();
            let mut request = [0_u8; 4096];
            let _ = stream.read(&mut request).unwrap();
            let http = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                response.len(),
                response
            );
            stream.write_all(http.as_bytes()).unwrap();
        }
    });
    url
}

fn signed_tx_hex_for_tests() -> String {
    let tx = bitcoin::Transaction {
        version: bitcoin::transaction::Version(2),
        lock_time: bitcoin::absolute::LockTime::ZERO,
        input: Vec::new(),
        output: Vec::new(),
    };
    bitcoin::consensus::encode::serialize_hex(&tx)
}
