use std::str::FromStr;
use tempfile::NamedTempFile;
use thornado_core::{
    apply_event, derive_split_receipt, execute_command, load_snapshot, load_snapshot_str,
    mine_deposit_pow, offline_penalty_sats, replay_events, required_node_bond_sats,
    required_node_bond_sats_for_state, required_node_bond_sats_with_params, save_snapshot,
    stark_field_from_bytes, stark_withdrawal_from_receipt, state_hash, withdrawal_from_receipt,
    AppState, Command, Error, Event, FrostCustodySigner, MockCustodySigner, MockProofVerifier,
    NodeStatus, ProofVerifier, StarkProofVerifier, WithdrawalRequest, BTC_SATS,
    DEFAULT_FEE_BUCKET_TARGET_SATS, SNAPSHOT_VERSION,
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
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = stark_withdrawal_from_receipt(
        note,
        "test-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .unwrap();

    execute_command(
        &mut state,
        Command::WithdrawNote {
            proof: proof.clone(),
            public: public.clone(),
        },
        &StarkProofVerifier,
        &signer,
    )
    .unwrap();

    assert!(state
        .notes
        .spent_nullifiers
        .contains(&public.nullifier_hash));
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
        &StarkProofVerifier,
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
    let first_intent = &state.deposits.intents["dep-1"];
    assert_eq!(first_intent.custody_epoch, 0);
    assert!(!first_intent.deposit_key_tweak.is_empty());
    assert!(!first_intent.deposit_public_key.is_empty());
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
fn standby_nodes_are_promoted_on_churn_before_keygen() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let registered = execute_command(
        &mut state,
        Command::RegisterStandbyNode {
            node_id: "node-1".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(matches!(
        registered.as_slice(),
        [Event::StandbyNodeRegistered { node_id }] if node_id == "node-1"
    ));
    assert!(state.churn.standby_nodes.contains("node-1"));
    assert!(!state.churn.active_nodes.contains("node-1"));

    let events = execute_command(&mut state, Command::StartChurnEpoch, &verifier, &signer).unwrap();

    assert!(events
        .iter()
        .any(|event| matches!(event, Event::StandbyNodeActivated { node_id, epoch: 1 } if node_id == "node-1")));
    assert!(state.churn.active_nodes.contains("node-1"));
    assert!(!state.churn.standby_nodes.contains("node-1"));
    assert!(!state.custody.keysets.contains_key(&1));
}

#[test]
fn standby_offline_nodes_are_not_penalized_until_active() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    for command in [
        Command::RegisterStandbyNode {
            node_id: "standby-1".to_string(),
        },
        Command::MarkNodeOffline {
            node_id: "standby-1".to_string(),
        },
        Command::ApplyChurnPenalties,
    ] {
        execute_command(&mut state, command, &verifier, &signer).unwrap();
    }

    assert!(state
        .churn
        .offline_nodes
        .get(&0)
        .unwrap()
        .contains("standby-1"));
    assert!(!state
        .churn
        .penalized_nodes
        .get(&0)
        .is_some_and(|nodes| nodes.contains("standby-1")));
}

#[test]
fn node_account_requires_bonded_slot_before_churn_in() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    execute_command(
        &mut state,
        Command::RegisterNode {
            node_id: "node-slot-0".to_string(),
            bond_address: "tbond1".to_string(),
            consensus_pubkey: "consensus-1".to_string(),
            signer_pubkey: "signer-1".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert_eq!(
        state.churn.node_accounts["node-slot-0"].status,
        NodeStatus::Standby
    );

    let err = execute_command(
        &mut state,
        Command::AssignNodeSlot {
            node_id: "node-slot-0".to_string(),
            slot_id: 0,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InsufficientNodeBond);

    execute_command(
        &mut state,
        Command::BondNode {
            node_id: "node-slot-0".to_string(),
            amount_sats: required_node_bond_sats(0),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::AssignNodeSlot {
            node_id: "node-slot-0".to_string(),
            slot_id: 0,
        },
        &verifier,
        &signer,
    )
    .unwrap();

    let events = execute_command(&mut state, Command::StartChurnEpoch, &verifier, &signer).unwrap();

    assert!(events.iter().any(|event| {
        matches!(
            event,
            Event::NodeStatusUpdated {
                node_id,
                status: NodeStatus::Ready,
                epoch: 1
            } if node_id == "node-slot-0"
        )
    }));
    assert!(events.iter().any(|event| {
        matches!(
            event,
            Event::StandbyNodeActivated { node_id, epoch: 1 } if node_id == "node-slot-0"
        )
    }));
    assert_eq!(
        state.churn.node_accounts["node-slot-0"].status,
        NodeStatus::Active
    );
    assert!(state.churn.active_nodes.contains("node-slot-0"));
}

#[test]
fn node_slot_assignment_is_permanent_and_unique() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    for node_id in ["node-a", "node-b"] {
        execute_command(
            &mut state,
            Command::RegisterNode {
                node_id: node_id.to_string(),
                bond_address: format!("bond-{node_id}"),
                consensus_pubkey: format!("consensus-{node_id}"),
                signer_pubkey: format!("signer-{node_id}"),
            },
            &verifier,
            &signer,
        )
        .unwrap();
        execute_command(
            &mut state,
            Command::BondNode {
                node_id: node_id.to_string(),
                amount_sats: required_node_bond_sats(1),
            },
            &verifier,
            &signer,
        )
        .unwrap();
    }

    execute_command(
        &mut state,
        Command::AssignNodeSlot {
            node_id: "node-a".to_string(),
            slot_id: 1,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let err = execute_command(
        &mut state,
        Command::AssignNodeSlot {
            node_id: "node-b".to_string(),
            slot_id: 1,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();

    assert_eq!(err, Error::NodeSlotAlreadyAssigned);
    assert_eq!(state.churn.node_slots[&1].owner_node_id, "node-a");
}

#[test]
fn bond_schedule_uses_state_parameters() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    assert_eq!(required_node_bond_sats(0), 10 * BTC_SATS);
    assert_eq!(required_node_bond_sats(1), 12 * BTC_SATS + BTC_SATS / 2);
    assert_eq!(required_node_bond_sats(2), 15 * BTC_SATS);

    execute_command(
        &mut state,
        Command::SetBondParameters {
            min_bond_sats: 1_000,
            min_bond_increase_sats: 250,
        },
        &verifier,
        &signer,
    )
    .unwrap();

    assert_eq!(required_node_bond_sats_with_params(1_000, 250, 0), 1_000);
    assert_eq!(required_node_bond_sats_for_state(&state, 0), 1_000);
    assert_eq!(required_node_bond_sats_for_state(&state, 1), 1_250);
    assert_eq!(required_node_bond_sats_for_state(&state, 2), 1_500);
}

#[test]
fn active_offline_node_accrues_slash_points_and_loses_one_percent_bond() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    let bond_sats = required_node_bond_sats(0);

    execute_command(
        &mut state,
        Command::RegisterNode {
            node_id: "slash-node".to_string(),
            bond_address: "bond-slash".to_string(),
            consensus_pubkey: "consensus-slash".to_string(),
            signer_pubkey: "signer-slash".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::BondNode {
            node_id: "slash-node".to_string(),
            amount_sats: bond_sats,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::AssignNodeSlot {
            node_id: "slash-node".to_string(),
            slot_id: 0,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(&mut state, Command::StartChurnEpoch, &verifier, &signer).unwrap();
    execute_command(
        &mut state,
        Command::MarkNodeOffline {
            node_id: "slash-node".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let events =
        execute_command(&mut state, Command::ApplyChurnPenalties, &verifier, &signer).unwrap();

    assert!(events.iter().any(|event| {
        matches!(
            event,
            Event::NodeSlashPointsAdded { node_id, points: 1, .. } if node_id == "slash-node"
        )
    }));
    assert!(events.iter().any(|event| {
        matches!(
            event,
            Event::NodeBondSlashed { node_id, amount_sats, .. }
                if node_id == "slash-node" && *amount_sats == offline_penalty_sats(bond_sats)
        )
    }));
    let node = &state.churn.node_accounts["slash-node"];
    assert_eq!(node.slash_points, 1);
    assert_eq!(node.missed_observations, 1);
    assert_eq!(node.missed_keysigns, 1);
    assert_eq!(node.bond_sats, bond_sats - offline_penalty_sats(bond_sats));
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
fn frost_coordinator_signs_for_unhardened_deposit_child_key() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let mut state = AppState::default();
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 0,
            keyset: signer.to_keyset(0).unwrap(),
        },
    )
    .unwrap();
    let child = thornado_core::derive_deposit_child_key(
        &state,
        "dep-1",
        &pow("child-key"),
        "test-client-pubkey",
    )
    .unwrap();
    let coordinator = signer.coordinator();
    let signer_nodes = signer.signer_nodes();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-child-key".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "nullifier-hash".to_string(),
    };

    let signature = coordinator
        .sign_with_child_tweak(&request, &signer_nodes[..4], &child.key_tweak)
        .unwrap();

    assert_eq!(signature.scheme, "frost-secp256k1-sha256+tweak");
    assert_eq!(signature.group_public_key, child.child_public_key);
    assert_eq!(
        signature.key_tweak.as_deref(),
        Some(child.key_tweak.as_str())
    );
    thornado_core::verify_custody_signature(&request, &signature).unwrap();
}

#[test]
fn frost_signer_snapshot_roundtrips_and_keeps_keysign_working() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let snapshot = signer.to_snapshot().unwrap();
    let restored = FrostCustodySigner::from_snapshot(&snapshot).unwrap();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-restored".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "nullifier-hash".to_string(),
    };

    assert_eq!(
        restored.group_public_key_hex().unwrap(),
        signer.group_public_key_hex().unwrap()
    );
    let signature =
        thornado_core::CustodySigner::authorize_withdrawal(&restored, &request).unwrap();
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

    state.churn.active_nodes.insert("node-a".to_string());
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
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = stark_withdrawal_from_receipt(
        note,
        "test-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .unwrap();
    let spent_nullifier = public.nullifier_hash.clone();
    execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &StarkProofVerifier,
        &signer,
    )
    .unwrap();

    let hash = state_hash(&state);
    let file = NamedTempFile::new().unwrap();
    save_snapshot(&state, file.path()).unwrap();
    let loaded = load_snapshot(file.path()).unwrap();

    assert_eq!(hash, state_hash(&loaded));
    assert!(loaded.notes.spent_nullifiers.contains(&spent_nullifier));
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
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = stark_withdrawal_from_receipt(
        note,
        "test-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .unwrap();
    log.extend(
        execute_command(
            &mut state,
            Command::WithdrawNote { proof, public },
            &StarkProofVerifier,
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
fn stark_withdrawal_authorizes_without_secret_bearing_proof() {
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
    assert!(stark_withdrawal_from_receipt(
        note,
        "wrong-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .is_err());
    let (proof, public) = stark_withdrawal_from_receipt(
        note,
        "test-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .unwrap();

    assert!(proof.nullifier.is_empty());
    assert!(proof.secret.is_empty());
    assert!(proof.commitment.is_empty());
    let proof_hex = proof
        .orchard
        .as_ref()
        .map(|proof| proof.proof_hex.as_str())
        .or_else(|| proof.stark.as_ref().map(|proof| proof.proof_hex.as_str()))
        .unwrap();
    let proof_bytes = hex::decode(proof_hex).unwrap();
    assert!(!proof_bytes
        .windows(note.nullifier.as_bytes().len())
        .any(|window| window == note.nullifier.as_bytes()));
    assert!(!proof_bytes
        .windows(note.secret.as_bytes().len())
        .any(|window| window == note.secret.as_bytes()));
    let proof_json = serde_json::to_string(&proof).unwrap();
    assert!(!proof_json.contains(&note.nullifier));
    assert!(!proof_json.contains(&note.secret));
    StarkProofVerifier
        .verify_withdrawal(&proof, &public)
        .unwrap();

    let mut tampered_recipient = public.clone();
    tampered_recipient.recipient = "tb1qtampered".to_string();
    assert!(StarkProofVerifier
        .verify_withdrawal(&proof, &tampered_recipient)
        .is_err());

    let mut tampered_recipient_field = public.clone();
    tampered_recipient_field.recipient = "tb1qtampered".to_string();
    tampered_recipient_field.recipient_field = Some(stark_field_from_bytes(
        tampered_recipient_field.recipient.as_bytes(),
    ));
    assert!(StarkProofVerifier
        .verify_withdrawal(&proof, &tampered_recipient_field)
        .is_err());

    let mut tampered_fee = public.clone();
    tampered_fee.fee_sats += 1;
    assert!(StarkProofVerifier
        .verify_withdrawal(&proof, &tampered_fee)
        .is_err());

    execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &StarkProofVerifier,
        &signer,
    )
    .unwrap();
}
