use tempfile::NamedTempFile;
use thornado_core::{
    derive_split_receipt, execute_command, load_snapshot, load_snapshot_str, nullifier_hash,
    replay_events, save_snapshot, state_hash, withdrawal_from_receipt, AppState, Command, Error,
    MockCustodySigner, MockProofVerifier, DEFAULT_FEE_BUCKET_TARGET_SATS, SNAPSHOT_VERSION,
};

fn confirmed_state(amount_sats: u64) -> AppState {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: "mock".to_string(),
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
            pow_token: "mock".to_string(),
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
            pow_token: "mock".to_string(),
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
