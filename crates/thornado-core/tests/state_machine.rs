use std::str::FromStr;
use tempfile::NamedTempFile;
use thornado_core::{
    apply_event, client_pubkey_from_secret, derive_split_receipt, execute_command, load_snapshot,
    load_snapshot_str, mine_deposit_pow, offline_penalty_sats, replay_events,
    required_node_bond_sats, required_node_bond_sats_for_state,
    required_node_bond_sats_with_params, save_snapshot, state_hash, withdrawal_from_receipt,
    zk_withdrawal_from_receipt, AppState, Command, Error, Event, FrostCustodySigner,
    MockCustodySigner, MockProofVerifier, NodeStatus, ProofVerifier, WithdrawalProof,
    WithdrawalPublicInputs, WithdrawalRequest, ZkProofVerifier, BTC_SATS,
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
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

fn mock_signature() -> thornado_core::CustodySignature {
    thornado_core::CustodySignature {
        scheme: "mock-sha256".to_string(),
        signer: "mock-frost-quorum".to_string(),
        message_digest: "digest".to_string(),
        group_public_key: "mock".to_string(),
        key_tweak: None,
        signature: "signature".to_string(),
    }
}

struct AcceptingHiddenVerifier;

impl ProofVerifier for AcceptingHiddenVerifier {
    fn verify_withdrawal(
        &self,
        _proof: &WithdrawalProof,
        _public: &WithdrawalPublicInputs,
    ) -> thornado_core::Result<()> {
        Ok(())
    }

    fn reveals_commitment(&self) -> bool {
        false
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-core --features proof-tests`"
)]
fn withdrawal_succeeds_once_and_charges_fee() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = zk_withdrawal_from_receipt(
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
        &ZkProofVerifier,
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
        &ZkProofVerifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::DuplicateNullifier);
}

#[test]
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-core --features proof-tests`"
)]
fn withdrawal_request_records_pending_then_authorization_spends_note() {
    let signer = FrostCustodySigner::demo_67_percent().unwrap();
    let mut state = confirmed_state(100_000_000);
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 0,
            keyset: signer.to_keyset(0).unwrap(),
        },
    )
    .unwrap();
    let verifier = ZkProofVerifier;
    execute_command(
        &mut state,
        split_command(100_000_000),
        &verifier,
        &MockCustodySigner,
    )
    .unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = zk_withdrawal_from_receipt(
        note,
        "test-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .unwrap();

    execute_command(
        &mut state,
        Command::RequestWithdrawal {
            proof,
            public: public.clone(),
        },
        &verifier,
        &MockCustodySigner,
    )
    .unwrap();

    assert!(state.withdrawals.pending.contains_key("wd-1"));
    assert!(!state
        .notes
        .spent_nullifiers
        .contains(&public.nullifier_hash));

    let request = WithdrawalRequest {
        withdrawal_id: "wd-1".to_string(),
        recipient: public.recipient,
        amount_sats: public.denomination_sats - public.fee_sats,
        fee_sats: public.fee_sats,
        nullifier_hash: public.nullifier_hash.clone(),
    };
    let signature = thornado_core::CustodySigner::authorize_withdrawal(&signer, &request).unwrap();
    execute_command(
        &mut state,
        Command::AuthorizeWithdrawal {
            withdrawal_id: "wd-1".to_string(),
            signature,
        },
        &verifier,
        &MockCustodySigner,
    )
    .unwrap();

    assert!(!state.withdrawals.pending.contains_key("wd-1"));
    assert!(state.withdrawals.authorized.contains_key("wd-1"));
    let outbound = state.withdrawals.bitcoin_outbounds.get("wd-1").unwrap();
    assert_eq!(outbound.recipient, request.recipient);
    assert_eq!(outbound.amount_sats, request.amount_sats);
    assert_eq!(outbound.scheduled_epoch, state.churn.epoch);
    assert!(outbound.published_txid.is_none());
    assert!(state
        .notes
        .spent_nullifiers
        .contains(&public.nullifier_hash));
}

#[test]
fn withdrawal_and_bitcoin_outbound_use_active_pool_keyset_after_churn() {
    let verifier = AcceptingHiddenVerifier;
    let signer = MockCustodySigner;
    let custody_signer = FrostCustodySigner::demo_67_percent().unwrap();
    let deposit_keyset = custody_signer.to_keyset(1).unwrap();
    let mut state = AppState::default();
    state.churn.active_nodes.extend(
        ["old-1", "old-2", "old-3", "old-4", "old-5"]
            .into_iter()
            .map(String::from),
    );
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 1,
            keyset: deposit_keyset,
        },
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("deposit-bound-keyset"),
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
            amount_sats: 100_000_000,
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
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    state.churn.active_nodes = ["new-1", "new-2", "new-3"]
        .into_iter()
        .map(String::from)
        .collect();
    let active_keyset = FrostCustodySigner::demo_67_percent()
        .unwrap()
        .to_keyset(2)
        .unwrap();
    let active_group_public_key = active_keyset.group_public_key.clone();
    let active_threshold = active_keyset.threshold;
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 2,
            keyset: active_keyset,
        },
    )
    .unwrap();

    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let proof = WithdrawalProof {
        nullifier: String::new(),
        secret: String::new(),
        commitment: String::new(),
        merkle_root: tree.root(),
        #[cfg(feature = "orchard-zcash")]
        orchard: None,
    };
    let public = WithdrawalPublicInputs {
        nullifier_hash: "deposit-bound-nullifier".to_string(),
        owner_pubkey: note.owner_pubkey.clone(),
        denomination_sats: note.denomination_sats,
        recipient: "tb1qrecipient".to_string(),
        fee_sats: 100_000,
        merkle_root: tree.root(),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    execute_command(
        &mut state,
        Command::RequestWithdrawal {
            proof,
            public: public.clone(),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    let pending = state.withdrawals.pending.get("wd-1").unwrap();
    assert_eq!(pending.custody_epoch, 2);
    assert!(pending.deposit_key_tweak.is_empty());
    assert_eq!(pending.vault_signers, vec!["new-1", "new-2", "new-3"]);
    assert_eq!(pending.vault_threshold, active_threshold);

    execute_command(
        &mut state,
        Command::AuthorizeWithdrawal {
            withdrawal_id: "wd-1".to_string(),
            signature: {
                let request = WithdrawalRequest {
                    withdrawal_id: "wd-1".to_string(),
                    recipient: public.recipient.clone(),
                    amount_sats: public.denomination_sats - public.fee_sats,
                    fee_sats: public.fee_sats,
                    nullifier_hash: public.nullifier_hash.clone(),
                };
                let mut signature =
                    thornado_core::CustodySigner::authorize_withdrawal(&signer, &request).unwrap();
                signature.group_public_key = active_group_public_key;
                signature
            },
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let outbound = state.withdrawals.bitcoin_outbounds.get("wd-1").unwrap();
    assert_eq!(outbound.custody_epoch, 2);
    assert!(outbound.deposit_key_tweak.is_empty());
    assert_eq!(outbound.vault_signers, vec!["new-1", "new-2", "new-3"]);
    assert_eq!(outbound.vault_threshold, active_threshold);
}

#[test]
fn repeated_payment_to_same_deposit_address_creates_new_credit() {
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    let mut state = confirmed_state(100_000_000);
    let original_address = state.deposits.intents["dep-1"].deposit_address.clone();

    let events = execute_command(
        &mut state,
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-2".to_string(),
            amount_sats: 50_000_000,
        },
        &verifier,
        &signer,
    )
    .unwrap();

    assert!(matches!(
        events.first(),
        Some(Event::DepositCreditCreated {
            credit_id,
            source_intent_id
        }) if credit_id == "dep-2" && source_intent_id == "dep-1"
    ));
    assert_eq!(
        state.deposits.intents["dep-2"].deposit_address,
        original_address
    );
    assert_eq!(
        state.deposits.intents["dep-2"].txid.as_deref(),
        Some("tx-2")
    );

    execute_command(
        &mut state,
        Command::ConfirmDeposit {
            intent_id: "dep-2".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-2".to_string(),
            note_commitments: derive_split_receipt("dep-2", 50_000_000, "test-seed")
                .unwrap()
                .commitments(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(state.deposits.intents["dep-2"].split);
}

#[test]
fn user_deposit_address_is_not_internal_vault_zero() {
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    let mut state = AppState::default();
    let events = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("not-vault-zero"),
            user_pubkey: client_pubkey_from_secret("test-seed"),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let deposit_address = events
        .iter()
        .find_map(|event| match event {
            Event::DepositIntentCreated {
                deposit_address, ..
            } => Some(deposit_address.clone()),
            _ => None,
        })
        .unwrap();
    let vault_zero =
        thornado_core::derive_vault_address(&state, state.custody.active_epoch, 0).unwrap();
    assert_ne!(deposit_address, vault_zero);
}

#[test]
fn bitcoin_solvency_requires_active_quorum_to_halt_and_unhalt() {
    let mut state = AppState::default();
    state
        .churn
        .active_nodes
        .extend(["node-1", "node-2"].into_iter().map(String::from));
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-1".to_string(),
            epoch: 0,
            expected_sats: 200,
            actual_sats: 100,
            spendable_sats: 100,
            solvent: false,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(!state.bitcoin_solvency.halted);

    execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-2".to_string(),
            epoch: 0,
            expected_sats: 200,
            actual_sats: 100,
            spendable_sats: 100,
            solvent: false,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(state.bitcoin_solvency.halted);

    execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-1".to_string(),
            epoch: 0,
            expected_sats: 100,
            actual_sats: 200,
            spendable_sats: 200,
            solvent: true,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(state.bitcoin_solvency.halted);

    execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-2".to_string(),
            epoch: 0,
            expected_sats: 100,
            actual_sats: 200,
            spendable_sats: 200,
            solvent: true,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(!state.bitcoin_solvency.halted);
}

#[test]
fn bitcoin_outbound_queue_tracks_authorized_withdrawals() {
    let mut state = AppState::default();
    state.churn.active_nodes.extend(
        ["node-1", "node-2", "node-3", "node-4"]
            .into_iter()
            .map(String::from),
    );
    apply_event(
        &mut state,
        Event::WithdrawalAuthorized {
            withdrawal_id: "wd-bitcoin".to_string(),
            recipient: "bcrt1qrecipient".to_string(),
            amount_sats: 100,
            fee_sats: 1,
            nullifier_hash: "nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: "deposit-tweak".to_string(),
            vault_signers: ["node-1", "node-2", "node-3", "node-4"]
                .into_iter()
                .map(String::from)
                .collect(),
            vault_threshold: 3,
            signature: mock_signature(),
        },
    )
    .unwrap();

    let outbound = state
        .withdrawals
        .bitcoin_outbounds
        .get("wd-bitcoin")
        .unwrap();
    assert_eq!(outbound.withdrawal_id, "wd-bitcoin");
    assert_eq!(outbound.recipient, "bcrt1qrecipient");
    assert_eq!(outbound.amount_sats, 100);
    assert_eq!(outbound.scheduled_epoch, 0);
    assert_eq!(outbound.signers.len(), 3);
    assert_eq!(outbound.attesters.len(), 4);
    assert!(outbound
        .leader
        .as_ref()
        .is_some_and(|leader| outbound.signers.contains(leader)));
    assert_eq!(outbound.signing_deadline_epoch, 1);
    assert_eq!(outbound.attestation_deadline_epoch, 1);
    assert!(outbound.published_txid.is_none());
}

#[test]
fn bitcoin_outbound_penalties_apply_once_after_deadlines() {
    let mut state = AppState::default();
    state.churn.epoch = 0;
    for node_id in ["node-1", "node-2", "node-3", "node-4"] {
        state.churn.active_nodes.insert(node_id.to_string());
        state.churn.node_accounts.insert(
            node_id.to_string(),
            thornado_core::NodeAccount {
                node_id: node_id.to_string(),
                status: NodeStatus::Active,
                ..Default::default()
            },
        );
    }
    apply_event(
        &mut state,
        Event::WithdrawalAuthorized {
            withdrawal_id: "wd-bitcoin".to_string(),
            recipient: "bcrt1qrecipient".to_string(),
            amount_sats: 100,
            fee_sats: 1,
            nullifier_hash: "nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: "deposit-tweak".to_string(),
            vault_signers: ["node-1", "node-2", "node-3", "node-4"]
                .into_iter()
                .map(String::from)
                .collect(),
            vault_threshold: 3,
            signature: mock_signature(),
        },
    )
    .unwrap();
    state.churn.epoch = 2;
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let events = execute_command(
        &mut state,
        Command::ApplyBitcoinOutboundPenalties,
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(events
        .iter()
        .any(|event| matches!(event, Event::NodeSlashPointsAdded { reason, .. } if reason == "missed_bitcoin_keysign")));
    assert!(events
        .iter()
        .any(|event| matches!(event, Event::NodeSlashPointsAdded { reason, .. } if reason == "missed_bitcoin_attestation")));

    let second = execute_command(
        &mut state,
        Command::ApplyBitcoinOutboundPenalties,
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(second.is_empty());
}

#[test]
fn deposit_intents_snapshot_vault_signers_and_retire_below_threshold() {
    let mut state = AppState::default();
    state.churn.active_nodes.extend(
        ["node-1", "node-2", "node-3", "node-4"]
            .into_iter()
            .map(String::from),
    );
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("retire-deposit"),
            user_pubkey: client_pubkey_from_secret("retire-seed"),
        },
        &verifier,
        &signer,
    )
    .unwrap();

    let intent = state.deposits.intents.get("dep-1").unwrap();
    assert_eq!(intent.vault_signers.len(), 4);
    assert_eq!(intent.vault_threshold, 3);

    state.churn.active_nodes.remove("node-3");
    let early = execute_command(
        &mut state,
        Command::ApplyDepositRetirements,
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(early.is_empty());
    assert!(state.deposits.intents.contains_key("dep-1"));

    state.churn.active_nodes.remove("node-4");
    let retired = execute_command(
        &mut state,
        Command::ApplyDepositRetirements,
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(matches!(
        retired.as_slice(),
        [Event::DepositIntentRetired {
            intent_id,
            reason,
            ..
        }] if intent_id == "dep-1" && reason == "vault_signers_below_threshold"
    ));
    assert!(!state.deposits.intents.contains_key("dep-1"));
}

#[test]
fn bitcoin_solvency_rejects_stale_epoch_and_inactive_reporter() {
    let mut state = AppState::default();
    state.churn.epoch = 2;
    state.churn.active_nodes.insert("node-1".to_string());
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let err = execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-1".to_string(),
            epoch: 1,
            expected_sats: 100,
            actual_sats: 100,
            spendable_sats: 100,
            solvent: true,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::StaleBitcoinSolvencyReport);

    let err = execute_command(
        &mut state,
        Command::SubmitBitcoinSolvency {
            reporter: "node-2".to_string(),
            epoch: 2,
            expected_sats: 100,
            actual_sats: 100,
            spendable_sats: 100,
            solvent: true,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidBitcoinSolvencyReporter);
}

#[test]
fn bitcoin_broadcast_requires_attestation_quorum() {
    let mut state = AppState::default();
    state
        .churn
        .active_nodes
        .extend(["node-1", "node-2"].into_iter().map(String::from));
    state.withdrawals.authorized.insert(
        "wd-bitcoin".to_string(),
        thornado_core::AuthorizedWithdrawal {
            id: "wd-bitcoin".to_string(),
            recipient: "bcrt1qrecipient".to_string(),
            amount_sats: 100,
            fee_sats: 1,
            nullifier_hash: "nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: "deposit-tweak".to_string(),
            vault_signers: Vec::new(),
            vault_threshold: 0,
            signature: mock_signature(),
        },
    );
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let err = execute_command(
        &mut state,
        Command::SubmitBitcoinBroadcast {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::MissingBitcoinAttestationQuorum);

    for node in ["node-1", "node-2"] {
        execute_command(
            &mut state,
            Command::AttestBitcoinWithdrawal {
                withdrawal_id: "wd-bitcoin".to_string(),
                txid: "txid".to_string(),
                signed_tx_hash: "signed-hash".to_string(),
                attester: node.to_string(),
                epoch: 0,
            },
            &verifier,
            &signer,
        )
        .unwrap();
    }

    execute_command(
        &mut state,
        Command::SubmitBitcoinBroadcast {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(state
        .withdrawals
        .bitcoin_broadcasts
        .contains_key("wd-bitcoin"));

    let duplicate = execute_command(
        &mut state,
        Command::SubmitBitcoinBroadcast {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    assert!(duplicate.is_empty());
}

#[test]
fn bitcoin_attestation_rejects_stale_inactive_and_conflicting_votes() {
    let mut state = AppState::default();
    state.churn.epoch = 1;
    state.churn.active_nodes.insert("node-1".to_string());
    state.withdrawals.authorized.insert(
        "wd-bitcoin".to_string(),
        thornado_core::AuthorizedWithdrawal {
            id: "wd-bitcoin".to_string(),
            recipient: "bcrt1qrecipient".to_string(),
            amount_sats: 100,
            fee_sats: 1,
            nullifier_hash: "nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: "deposit-tweak".to_string(),
            vault_signers: Vec::new(),
            vault_threshold: 0,
            signature: mock_signature(),
        },
    );
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;

    let err = execute_command(
        &mut state,
        Command::AttestBitcoinWithdrawal {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
            signed_tx_hash: "signed-hash".to_string(),
            attester: "node-1".to_string(),
            epoch: 0,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::StaleBitcoinAttestation);

    let err = execute_command(
        &mut state,
        Command::AttestBitcoinWithdrawal {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
            signed_tx_hash: "signed-hash".to_string(),
            attester: "node-2".to_string(),
            epoch: 1,
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidBitcoinAttestationReporter);

    execute_command(
        &mut state,
        Command::AttestBitcoinWithdrawal {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "txid".to_string(),
            signed_tx_hash: "signed-hash".to_string(),
            attester: "node-1".to_string(),
            epoch: 1,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let err = apply_event(
        &mut state,
        Event::BitcoinWithdrawalAttested {
            withdrawal_id: "wd-bitcoin".to_string(),
            txid: "other-txid".to_string(),
            signed_tx_hash: "other-hash".to_string(),
            attester: "node-1".to_string(),
            epoch: 1,
        },
    )
    .unwrap_err();
    assert_eq!(err, Error::ConflictingBitcoinAttestation);
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let second = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("unique-2"),
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
            user_pubkey: client_pubkey_from_secret("test-seed"),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let err = execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: token,
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-core --features proof-tests`"
)]
fn snapshot_roundtrip_preserves_hash_and_spend_status() {
    let mut state = confirmed_state(100_000_000);
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(&mut state, split_command(100_000_000), &verifier, &signer).unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
    let note = receipt.notes.first().unwrap();
    let tree = state.notes.trees.get(&100_000_000).unwrap();
    let (proof, public) = zk_withdrawal_from_receipt(
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
        &ZkProofVerifier,
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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-core --features proof-tests`"
)]
fn command_events_replay_to_the_same_state() {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    let mut log = Vec::new();

    for command in [
        Command::RequestDepositAddress {
            pow_token: pow("replay"),
            user_pubkey: client_pubkey_from_secret("test-seed"),
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
    let (proof, public) = zk_withdrawal_from_receipt(
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
            &ZkProofVerifier,
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

    let wrong_receipt = derive_split_receipt("dep-1", 100_000_000, "private-client-seed").unwrap();
    let err = execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-1".to_string(),
            note_commitments: wrong_receipt.commitments(),
        },
        &verifier,
        &signer,
    )
    .unwrap_err();
    assert_eq!(err, Error::InvalidNoteOwner);

    let receipt = derive_split_receipt("dep-1", 100_000_000, "test-seed").unwrap();
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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-core --features proof-tests`"
)]
fn zk_withdrawal_authorizes_without_secret_bearing_proof() {
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
    assert!(zk_withdrawal_from_receipt(
        note,
        "wrong-seed",
        tree,
        "tb1qrecipient".to_string(),
        100_000,
    )
    .is_err());
    let (proof, public) = zk_withdrawal_from_receipt(
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
    ZkProofVerifier.verify_withdrawal(&proof, &public).unwrap();

    let mut tampered_recipient = public.clone();
    tampered_recipient.recipient = "tb1qtampered".to_string();
    assert!(ZkProofVerifier
        .verify_withdrawal(&proof, &tampered_recipient)
        .is_err());

    let mut tampered_fee = public.clone();
    tampered_fee.fee_sats += 1;
    assert!(ZkProofVerifier
        .verify_withdrawal(&proof, &tampered_fee)
        .is_err());

    execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &ZkProofVerifier,
        &signer,
    )
    .unwrap();
}

#[cfg(feature = "orchard-zcash")]
#[test]
fn orchard_public_nullifier_must_match_proof_action_nullifier() {
    let action = thornado_core::orchard::OrchardActionPayload {
        nullifier_hex: "11".repeat(32),
        rk_hex: String::new(),
        cmx_hex: String::new(),
        cv_net_hex: String::new(),
        epk_hex: String::new(),
        enc_ciphertext_hex: String::new(),
        out_ciphertext_hex: String::new(),
        spend_auth_sig_hex: String::new(),
    };
    let proof = WithdrawalProof {
        nullifier: String::new(),
        secret: String::new(),
        commitment: String::new(),
        merkle_root: "22".repeat(32),
        orchard: Some(thornado_core::orchard::OrchardWithdrawalProof {
            proof_hex: String::new(),
            binding_signature_hex: String::new(),
            anchor_hex: "22".repeat(32),
            public_context_hex: String::new(),
            value_balance: 1_000_000,
            actions: vec![action],
        }),
    };
    let public = WithdrawalPublicInputs {
        nullifier_hash: "33".repeat(32),
        owner_pubkey: String::new(),
        denomination_sats: 1_000_000,
        recipient: "tb1qrecipient".to_string(),
        fee_sats: 0,
        merkle_root: "22".repeat(32),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };

    assert_eq!(
        ZkProofVerifier.verify_withdrawal(&proof, &public),
        Err(Error::InvalidProof)
    );
}
