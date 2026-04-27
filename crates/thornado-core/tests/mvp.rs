use std::str::FromStr;
use tempfile::NamedTempFile;
use thornado_core::{
    derive_split_receipt, execute_command, load_snapshot, load_snapshot_str, mine_deposit_pow,
    nullifier_hash, replay_events, save_snapshot, stark_withdrawal_from_receipt, state_hash,
    withdrawal_from_receipt, AppState, Command, Error, Event, FrostCustodySigner,
    MockCustodySigner, MockProofVerifier, WithdrawalRequest, DEFAULT_FEE_BUCKET_TARGET_SATS,
    SNAPSHOT_VERSION,
};

fn confirmed_state(amount_sats: u64) -> AppState {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("confirmed-state"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-1".to_string(),
            amount_sats,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::ConfirmDeposit {
            intent_id: "dep-1".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    state
}

fn split_command(amount_sats: u64) -> Command {
    Command::SplitDepositIntoNotes {
        deposit_id: "dep-1".to_string(),
        note_commitments: derive_split_receipt("dep-1", amount_sats, "test-seed")
            .unwrap()
            .commitments(),
    }
}

#[test]
fn deposit_cannot_split_before_confirmation() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("cannot-split"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    let err =
        execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap_err();

    assert_eq!(err, Error::DepositNotConfirmed);
}

#[test]
fn confirmed_deposit_splits_greedily_and_updates_trees() {
    let mut state = confirmed_state(1_111_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(1_111_000_000), &verifier, &signer).unwrap();

    assert_eq!(
        state.notes.trees.get(&1_000_000_000).unwrap().leaves.len(),
        1
    );
    assert_eq!(state.notes.trees.get(&100_000_000).unwrap().leaves.len(), 1);
    assert_eq!(state.notes.trees.get(&10_000_000).unwrap().leaves.len(), 1);
    assert_eq!(state.notes.trees.get(&1_000_000).unwrap().leaves.len(), 1);
    assert_eq!(
        state.deposits.intents.get("dep-1").unwrap().remainder_sats,
        0
    );
}

#[test]
fn withdrawal_succeeds_once_and_charges_fee() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, public) = withdrawal_from_receipt(note, root, "tb1qrecipient".to_string(), 100_000);

    execute_command(
        &mut state,
        Command::WithdrawNote {
            proof: proof.clone(),
            public: public.clone(),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    assert!(state
        .notes
        .spent_nullifiers
        .contains(&nullifier_hash(&note.nullifier)));
    assert_eq!(state.fees.total_collected_sats, 100_000);
    assert_eq!(state.withdrawals.authorized.len(), 1);
    let withdrawal = state.withdrawals.authorized.get("wd-1").unwrap();
    let request = WithdrawalRequest {
        withdrawal_id: withdrawal.id.clone(),
        recipient: withdrawal.recipient.clone(),
        amount_sats: withdrawal.amount_sats,
        fee_sats: withdrawal.fee_sats,
        nullifier_hash: withdrawal.nullifier_hash.clone(),
    };
    thornado_core::verify_custody_signature(&request, &withdrawal.signature).unwrap();

    let err = execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::DuplicateNullifier);
}

#[test]
fn deposit_addresses_are_unique_for_the_active_keyset() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let first = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("unique-1"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let second = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("unique-2"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    assert!(matches!(
        first.first(),
        Some(Event::CustodyKeysetGenerated { epoch: 0, .. })
    ));
    assert_eq!(state.custody.keysets.len(), 1);
    assert_ne!(
        state.deposits.intents["dep-1"].deposit_address,
        state.deposits.intents["dep-2"].deposit_address
    );
    let address = bitcoin::Address::from_str(&state.deposits.intents["dep-1"].deposit_address)
        .unwrap()
        .require_network(bitcoin::Network::Regtest)
        .unwrap();
    assert!(address.script_pubkey().is_p2tr());
    assert!(!second
        .iter()
        .any(|event| matches!(event, Event::CustodyKeysetGenerated { .. })));
}

#[test]
fn churn_generates_a_new_active_keyset_for_future_deposits() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("churn-1"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let old_public_key = state.custody.active_group_public_key.clone();
    let old_address = state.deposits.intents["dep-1"].deposit_address.clone();

    let churn_events =
        execute_command(&mut state, Command::StartChurnEpoch, &verifier, &signer).unwrap();
    assert!(matches!(
        churn_events.as_slice(),
        [
            Event::ChurnEpochStarted { epoch: 1 },
            Event::CustodyKeysetGenerated { epoch: 1, .. }
        ]
    ));
    assert_ne!(old_public_key, state.custody.active_group_public_key);

    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("churn-2"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert_ne!(old_address, state.deposits.intents["dep-2"].deposit_address);
    assert_eq!(state.custody.keysets.len(), 2);
}

#[test]
fn deposit_request_requires_valid_single_use_pow() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let err = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: "not-mined".to_string(),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidDepositPow);
    assert!(state.custody.keysets.is_empty());

    let token = pow("single-use");
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: token.clone(),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let err = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: token,
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::DuplicateDepositPow);
}

#[test]
fn frost_dkg_signer_authorizes_with_verifiable_threshold_signature() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-frost".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "nullifier-hash".to_string(),
    };

    let signature = thornado_core::CustodySigner::authorize_withdrawal(&signer, &request).unwrap();

    assert_eq!(signature.scheme, "frost-secp256k1-sha256");
    assert_eq!(signature.signer, "frost-4-of-5");
    thornado_core::verify_custody_signature(&request, &signature).unwrap();
}

#[test]
fn frost_coordinator_signs_with_four_of_five_separate_signer_nodes() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let coordinator = signer.coordinator();
    let signer_nodes = signer.signer_nodes();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-4-of-5".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "nullifier-hash".to_string(),
    };

    assert_eq!(signer_nodes.len(), 5);
    let signature = coordinator
        .sign_with_nodes(&request, &signer_nodes[..4])
        .unwrap();

    assert_eq!(signature.signer, "frost-4-of-5");
    thornado_core::verify_custody_signature(&request, &signature).unwrap();
}

#[test]
fn frost_coordinator_rejects_fewer_than_threshold_signer_nodes() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let coordinator = signer.coordinator();
    let signer_nodes = signer.signer_nodes();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-3-of-5".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "nullifier-hash".to_string(),
    };

    let err = coordinator
        .sign_with_nodes(&request, &signer_nodes[..3])
        .unwrap_err();
    assert!(matches!(err, Error::Frost(message) if message.contains("insufficient FROST signers")));
}

#[test]
fn withdrawal_rejects_unknown_root_wrong_denomination_and_duplicate_nullifier() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, mut public) =
        withdrawal_from_receipt(note, root, "tb1qrecipient".to_string(), 100_000);

    public.merkle_root = "unknown-root".to_string();
    let err = execute_command(
        &mut state,
        Command::WithdrawNote {
            proof: proof.clone(),
            public: public.clone(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidProof);

    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, mut public) =
        withdrawal_from_receipt(note, root, "tb1qrecipient".to_string(), 100_000);
    public.denomination_sats = 10_000_000;
    let err = execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidProof);
}

#[test]
fn fee_bucket_seals_when_target_is_reached() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    thornado_core::apply_event(
        &mut state,
        thornado_core::Event::FeeCharged {
            amount_sats: DEFAULT_FEE_BUCKET_TARGET_SATS + 7,
        },
    )
    .unwrap();

    assert_eq!(state.fees.sealed_buckets, 1);
    assert_eq!(state.fees.current_bucket_sats, 7);
    assert_eq!(
        execute_command(&mut state, Command::ApplyChurnPenalties, &verifier, &signer).unwrap(),
        Vec::new()
    );
}

#[test]
fn churn_epoch_and_offline_penalties_are_deterministic_and_do_not_break_notes() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let before_root = state.notes.trees.get(&100_000_000).unwrap().root();

    execute_command(&mut state, Command::StartChurnEpoch, &verifier, &signer).unwrap();
    execute_command(
        &mut state,
        Command::MarkNodeOffline {
            node_id: "node-a".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(&mut state, Command::ApplyChurnPenalties, &verifier, &signer).unwrap();

    assert_eq!(state.churn.epoch, 1);
    assert!(state
        .churn
        .penalized_nodes
        .get(&1)
        .unwrap()
        .contains("node-a"));
    assert_eq!(
        before_root,
        state.notes.trees.get(&100_000_000).unwrap().root()
    );
}

#[test]
fn snapshot_roundtrip_preserves_hash_and_spend_status() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, public) = withdrawal_from_receipt(note, root, "tb1qrecipient".to_string(), 100_000);
    execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &verifier,
        &signer,
    )
    .unwrap();

    let hash = state_hash(&state);
    let file = NamedTempFile::new().unwrap();
    save_snapshot(&state, file.path()).unwrap();
    let loaded = load_snapshot(file.path()).unwrap();

    assert_eq!(hash, state_hash(&loaded));
    assert!(loaded
        .notes
        .spent_nullifiers
        .contains(&nullifier_hash(&note.nullifier)));
}

#[test]
fn command_events_replay_to_the_same_state() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    let mut log = Vec::new();

    for command in [
        Command::RequestDepositAddress {
            pow_token: pow("replay"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-1".to_string(),
            amount_sats: 100_000_000,
        },
        Command::ConfirmDeposit {
            intent_id: "dep-1".to_string(),
        },
        split_command(100_000_000),
        Command::StartChurnEpoch,
        Command::MarkNodeOffline {
            node_id: "node-a".to_string(),
        },
        Command::ApplyChurnPenalties,
    ] {
        log.extend(execute_command(&mut state, command, &verifier, &signer).unwrap());
    }

    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, public) = withdrawal_from_receipt(note, root, "tb1qrecipient".to_string(), 100_000);
    log.extend(
        execute_command(
            &mut state,
            Command::WithdrawNote { proof, public },
            &verifier,
            &signer,
        )
        .unwrap(),
    );

    let replayed = replay_events(log).unwrap();
    assert_eq!(state, replayed);
    assert_eq!(state_hash(&state), state_hash(&replayed));
}

fn pow(label: &str) -> String {
    mine_deposit_pow(label)
}

#[test]
fn snapshot_is_versioned_and_rejects_unsupported_versions() {
    let state = confirmed_state(100_000_000);
    let file = NamedTempFile::new().unwrap();
    save_snapshot(&state, file.path()).unwrap();

    let json = std::fs::read_to_string(file.path()).unwrap();
    let value: serde_json::Value = serde_json::from_str(&json).unwrap();
    assert_eq!(
        value.get("version").and_then(|version| version.as_u64()),
        Some(SNAPSHOT_VERSION as u64)
    );
    assert!(value.get("state").is_some());

    let err = load_snapshot_str(r#"{"version":999,"state":{}}"#).unwrap_err();
    assert_eq!(err, Error::UnsupportedSnapshotVersion(999));
}

#[test]
fn split_requires_client_supplied_commitments_and_does_not_store_receipt_secrets() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let err = execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-1".to_string(),
            note_commitments: Vec::new(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidNoteCommitments);

    let receipt = derive_split_receipt("dep-1", 100_000_000, "private-client-seed").unwrap();
    execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-1".to_string(),
            note_commitments: receipt.commitments(),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    let state_json = serde_json::to_string(&state).unwrap();
    assert!(!state_json.contains(&receipt.notes[0].nullifier));
    assert!(!state_json.contains(&receipt.notes[0].secret));
    assert!(state_json.contains(&receipt.notes[0].commitment));
}

#[test]
fn unsafe_stark_withdrawal_backend_is_disabled() {
    let mut state = confirmed_state(100_000_000);
    let mock_verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        split_command(100_000_000),
        &mock_verifier,
        &signer,
    )
    .unwrap();

    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let err = stark_withdrawal_from_receipt(note, tree, "tb1qrecipient".to_string(), 100_000)
        .unwrap_err();
    assert!(matches!(err, Error::Stark(_)));
}
