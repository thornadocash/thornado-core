use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, CompressedPublicKey, Network};
use thornado_bitcoin::{
    script_hex, tx_bytes, txid_for_tests, BitcoinBackend, BitcoinWithdrawalRequest,
    DevBitcoinBackend, Error, RegtestUtxo, RegtestVault,
};

#[test]
fn builds_unsigned_regtest_withdrawal_from_imported_utxo() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(RegtestUtxo {
            txid: txid_for_tests(1),
            vout: 0,
            value_sats: 150_000,
            script_pubkey_hex: change_script_hex(),
        })
        .unwrap();

    let built = vault
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-1".to_string(),
            recipient: recipient(Network::Regtest),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: Some(change_script_hex()),
        })
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
        .import_utxo(RegtestUtxo {
            txid: txid_for_tests(2),
            vout: 0,
            value_sats: 150_000,
            script_pubkey_hex: change_script_hex(),
        })
        .unwrap();

    let err = vault
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-1".to_string(),
            recipient: recipient(Network::Bitcoin),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: Some(change_script_hex()),
        })
        .unwrap_err();

    assert_eq!(err, Error::NonRegtestNetwork);
}

#[test]
fn rejects_insufficient_funds() {
    let mut vault = RegtestVault::default();
    vault
        .import_utxo(RegtestUtxo {
            txid: txid_for_tests(3),
            vout: 0,
            value_sats: 20_000,
            script_pubkey_hex: change_script_hex(),
        })
        .unwrap();

    let err = vault
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-1".to_string(),
            recipient: recipient(Network::Regtest),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: Some(change_script_hex()),
        })
        .unwrap_err();

    assert!(matches!(err, Error::InsufficientFunds { .. }));
}

#[test]
fn dev_backend_records_lifecycle_and_reserves_utxos() {
    let mut backend = DevBitcoinBackend::new();
    backend
        .import_dev_utxo(RegtestUtxo {
            txid: txid_for_tests(4),
            vout: 0,
            value_sats: 150_000,
            script_pubkey_hex: change_script_hex(),
        })
        .unwrap();

    let built = backend
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-1".to_string(),
            recipient: recipient(Network::Regtest),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: Some(change_script_hex()),
        })
        .unwrap();
    assert_eq!(built.withdrawal_id, "wd-1");

    let err = backend
        .build_withdrawal(BitcoinWithdrawalRequest {
            withdrawal_id: "wd-2".to_string(),
            recipient: recipient(Network::Regtest),
            amount_sats: 100_000,
            fee_rate_sats_per_vb: 2,
            change_script_pubkey_hex: Some(change_script_hex()),
        })
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

fn change_script_hex() -> String {
    script_hex(&address(Network::Regtest).script_pubkey())
}

fn recipient(network: Network) -> String {
    address(network).to_string()
}

fn address(network: Network) -> Address {
    let secp = Secp256k1::new();
    let secret = SecretKey::from_slice(&[1_u8; 32]).unwrap();
    let public_key = CompressedPublicKey(SecpPublicKey::from_secret_key(&secp, &secret));
    Address::p2wpkh(&public_key, network)
}
