//! Orchard-backed shielded note primitives.
//!
//! This module intentionally uses the Zcash `orchard` crate directly. It is the
//! migration target for replacing the current MVP `stark` module with
//! Orchard/Halo2 notes, anchors, nullifiers, and spend authorization.

use crate::{Error, Result};
use incrementalmerkletree::{Hashable, Marking, Retention};
use nonempty::NonEmpty;
use orchard::builder::{Builder, BundleType};
use orchard::bundle::{Authorized, Flags};
use orchard::circuit::{ProvingKey, VerifyingKey};
use orchard::keys::{
    FullViewingKey, PreparedIncomingViewingKey, Scope, SpendAuthorizingKey, SpendingKey,
};
use orchard::note::{ExtractedNoteCommitment, Nullifier, TransmittedNoteCiphertext};
use orchard::note_encryption::OrchardDomain;
use orchard::primitives::redpallas::{self, Binding, SpendAuth};
use orchard::tree::MerkleHashOrchard;
use orchard::value::{NoteValue, ValueCommitment};
use orchard::{Action, Anchor, Bundle, Proof};
use rand_chacha::ChaCha20Rng;
use rand_core::OsRng;
use rand_core::SeedableRng;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use shardtree::{store::memory::MemoryShardStore, ShardTree};
use zcash_note_encryption::try_note_decryption;

pub const ORCHARD_NOTE_COMMITMENT_TREE_DEPTH: usize = orchard::NOTE_COMMITMENT_TREE_DEPTH;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct OrchardNoteReceipt {
    pub output_action: OrchardActionPayload,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct OrchardWithdrawalProof {
    pub proof_hex: String,
    pub binding_signature_hex: String,
    pub anchor_hex: String,
    pub public_context_hex: String,
    pub value_balance: i64,
    pub actions: Vec<OrchardActionPayload>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct OrchardActionPayload {
    pub nullifier_hex: String,
    pub rk_hex: String,
    pub cmx_hex: String,
    pub cv_net_hex: String,
    pub epk_hex: String,
    pub enc_ciphertext_hex: String,
    pub out_ciphertext_hex: String,
    pub spend_auth_sig_hex: String,
}

pub fn spending_key_from_seed(seed: &str) -> Result<SpendingKey> {
    for counter in 0_u32..u32::MAX {
        let digest = Sha256::digest([seed.as_bytes(), &counter.to_le_bytes()].concat());
        let mut bytes = [0_u8; 32];
        bytes.copy_from_slice(&digest);
        if let Some(sk) = Option::<SpendingKey>::from(SpendingKey::from_bytes(bytes)) {
            return Ok(sk);
        }
    }
    Err(Error::InvalidProof)
}

pub fn raw_address_from_seed(seed: &str) -> Result<[u8; 43]> {
    let sk = spending_key_from_seed(seed)?;
    let fvk = FullViewingKey::from(&sk);
    Ok(fvk
        .address_at(0_u32, Scope::External)
        .to_raw_address_bytes())
}

pub fn create_orchard_note(
    seed: &str,
    deposit_id: &str,
    index: u64,
    value_sats: u64,
) -> Result<(String, OrchardNoteReceipt)> {
    let mut rng = ChaCha20Rng::from_seed(deterministic_note_rng_seed(
        seed, deposit_id, index, value_sats,
    ));
    let sk = spending_key_from_seed(seed)?;
    let fvk = FullViewingKey::from(&sk);
    let recipient = fvk.address_at(0_u32, Scope::External);
    let anchor = MerkleHashOrchard::empty_root(32.into()).into();
    let mut builder = Builder::new(
        BundleType::Transactional {
            flags: Flags::SPENDS_DISABLED,
            bundle_required: false,
        },
        anchor,
    );
    builder
        .add_output(
            None,
            recipient,
            NoteValue::from_raw(value_sats),
            [0_u8; 512],
        )
        .map_err(|e| Error::Stark(format!("Orchard output build failed: {e:?}")))?;
    let (unauthorized, meta): (Bundle<_, i64>, _) = builder
        .build(&mut rng)
        .map_err(|e| Error::Stark(format!("Orchard shielding bundle build failed: {e:?}")))?
        .ok_or(Error::InvalidProof)?;
    let output_index = meta.output_action_index(0).ok_or(Error::InvalidProof)?;

    let action = unauthorized
        .actions()
        .get(output_index)
        .ok_or(Error::InvalidProof)?;
    let (note, _, _) = unauthorized
        .decrypt_output_with_key(output_index, &fvk.to_ivk(Scope::External))
        .ok_or(Error::InvalidProof)?;
    let cmx = hex::encode(ExtractedNoteCommitment::from(note.commitment()).to_bytes());
    Ok((
        cmx,
        OrchardNoteReceipt {
            output_action: action_to_public_payload(action),
        },
    ))
}

fn deterministic_note_rng_seed(
    seed: &str,
    deposit_id: &str,
    index: u64,
    value_sats: u64,
) -> [u8; 32] {
    let mut hasher = Sha256::new();
    hasher.update(b"thornado:orchard-note-rng");
    hasher.update((seed.len() as u64).to_be_bytes());
    hasher.update(seed.as_bytes());
    hasher.update((deposit_id.len() as u64).to_be_bytes());
    hasher.update(deposit_id.as_bytes());
    hasher.update(index.to_be_bytes());
    hasher.update(value_sats.to_be_bytes());
    hasher.finalize().into()
}

pub fn prove_orchard_withdrawal(
    seed: &str,
    receipt: &OrchardNoteReceipt,
    leaves: &[String],
    commitment: &str,
    public_context: &[u8],
) -> Result<(OrchardWithdrawalProof, String, String)> {
    let mut rng = OsRng;
    let pk = ProvingKey::build();
    let vk = VerifyingKey::build();
    let sk = spending_key_from_seed(seed)?;
    let fvk = FullViewingKey::from(&sk);
    let ivk = PreparedIncomingViewingKey::new(&fvk.to_ivk(Scope::External));
    let output_action = payload_to_output_action(&receipt.output_action)?;
    let (note, _, _) = {
        let domain = OrchardDomain::for_action(&output_action);
        try_note_decryption(&domain, &ivk, &output_action).ok_or(Error::InvalidProof)?
    };
    let cmx: ExtractedNoteCommitment = note.commitment().into();
    if hex::encode(cmx.to_bytes()) != commitment {
        return Err(Error::InvalidProof);
    }

    let (root, merkle_path) = orchard_path(leaves, commitment)?;
    let mut builder = Builder::new(
        BundleType::Transactional {
            flags: Flags::OUTPUTS_DISABLED,
            bundle_required: false,
        },
        root.into(),
    );
    builder
        .add_spend(fvk, note, merkle_path.into())
        .map_err(|e| Error::Stark(format!("Orchard spend build failed: {e:?}")))?;
    let (unauthorized, meta): (Bundle<_, i64>, _) = builder
        .build(&mut rng)
        .map_err(|e| Error::Stark(format!("Orchard spend bundle build failed: {e:?}")))?
        .ok_or(Error::InvalidProof)?;
    let spend_index = meta.spend_action_index(0).ok_or(Error::InvalidProof)?;
    let sighash = orchard_sighash(
        unauthorized.commitment().into(),
        &[b"thornado:withdraw".as_slice(), public_context].concat(),
    );
    let proven = unauthorized
        .create_proof(&pk, &mut rng)
        .map_err(|e| Error::Stark(format!("Orchard spend proof failed: {e:?}")))?;
    let spend_bundle = proven
        .apply_signatures(&mut rng, sighash, &[SpendAuthorizingKey::from(&sk)])
        .map_err(|e| Error::Stark(format!("Orchard spend signature failed: {e:?}")))?;
    verify_bundle(
        &spend_bundle,
        &vk,
        &[b"thornado:withdraw".as_slice(), public_context].concat(),
    )?;
    let spend_action = spend_bundle
        .actions()
        .get(spend_index)
        .ok_or(Error::InvalidProof)?;
    let nullifier_hex = hex::encode(spend_action.nullifier().to_bytes());
    Ok((
        bundle_to_payload(&spend_bundle, public_context),
        nullifier_hex,
        hex::encode(root.to_bytes()),
    ))
}

pub fn verify_orchard_withdrawal(
    proof: &OrchardWithdrawalProof,
    public_context: &[u8],
) -> Result<()> {
    if proof.public_context_hex != hex::encode(public_context) {
        return Err(Error::InvalidProof);
    }
    let vk = VerifyingKey::build();
    let bundle = payload_to_bundle(proof)?;
    verify_bundle(
        &bundle,
        &vk,
        &[b"thornado:withdraw".as_slice(), public_context].concat(),
    )
}

pub fn merkle_root_hex(leaves: &[String]) -> Result<String> {
    if leaves.is_empty() {
        let empty = MerkleHashOrchard::empty_root(32.into());
        return Ok(hex::encode(empty.to_bytes()));
    }
    let mut tree: ShardTree<MemoryShardStore<MerkleHashOrchard, u32>, 32, 16> =
        ShardTree::new(MemoryShardStore::empty(), 100);
    for leaf in leaves {
        tree.append(
            parse_merkle_hash(leaf)?,
            Retention::Checkpoint {
                id: 0,
                marking: Marking::Marked,
            },
        )
        .map_err(|e| Error::Stark(format!("Orchard tree append failed: {e:?}")))?;
    }
    let root = tree
        .root_at_checkpoint_id(&0)
        .map_err(|e| Error::Stark(format!("Orchard root lookup failed: {e:?}")))?
        .ok_or(Error::UnknownMerkleRoot)?;
    Ok(hex::encode(root.to_bytes()))
}

pub fn prove_orchard_spend_smoke(seed: &str, value_sats: u64) -> Result<Vec<u8>> {
    let mut rng = OsRng;
    let pk = ProvingKey::build();
    let vk = VerifyingKey::build();
    let sk = spending_key_from_seed(seed)?;
    let fvk = FullViewingKey::from(&sk);
    let recipient = fvk.address_at(0_u32, Scope::External);

    let shielding_bundle: Bundle<_, i64> = {
        let anchor = MerkleHashOrchard::empty_root(32.into()).into();
        let mut builder = Builder::new(
            BundleType::Transactional {
                flags: Flags::SPENDS_DISABLED,
                bundle_required: false,
            },
            anchor,
        );
        builder
            .add_output(
                None,
                recipient,
                NoteValue::from_raw(value_sats),
                [0_u8; 512],
            )
            .map_err(|e| Error::Stark(format!("Orchard output build failed: {e:?}")))?;
        let (unauthorized, _) = builder
            .build(&mut rng)
            .map_err(|e| Error::Stark(format!("Orchard shielding bundle build failed: {e:?}")))?
            .ok_or(Error::InvalidProof)?;
        let sighash = orchard_sighash(unauthorized.commitment().into(), b"thornado:shield");
        let proven = unauthorized
            .create_proof(&pk, &mut rng)
            .map_err(|e| Error::Stark(format!("Orchard shielding proof failed: {e:?}")))?;
        proven
            .apply_signatures(&mut rng, sighash, &[])
            .map_err(|e| Error::Stark(format!("Orchard shielding signature failed: {e:?}")))?
    };
    verify_bundle(&shielding_bundle, &vk, b"thornado:shield")?;

    let spend_bundle: Bundle<_, i64> = {
        let ivk = PreparedIncomingViewingKey::new(&fvk.to_ivk(Scope::External));
        let (note, _, _) = shielding_bundle
            .actions()
            .iter()
            .find_map(|action| {
                let domain = OrchardDomain::for_action(action);
                try_note_decryption(&domain, &ivk, action)
            })
            .ok_or(Error::InvalidProof)?;

        let cmx: ExtractedNoteCommitment = note.commitment().into();
        let leaf = MerkleHashOrchard::from_cmx(&cmx);
        let mut tree: ShardTree<MemoryShardStore<MerkleHashOrchard, u32>, 32, 16> =
            ShardTree::new(MemoryShardStore::empty(), 100);
        tree.append(
            leaf,
            Retention::Checkpoint {
                id: 0,
                marking: Marking::Marked,
            },
        )
        .map_err(|e| Error::Stark(format!("Orchard tree append failed: {e:?}")))?;
        let root = tree
            .root_at_checkpoint_id(&0)
            .map_err(|e| Error::Stark(format!("Orchard root lookup failed: {e:?}")))?
            .ok_or(Error::UnknownMerkleRoot)?;
        let position = tree
            .max_leaf_position(None)
            .map_err(|e| Error::Stark(format!("Orchard position lookup failed: {e:?}")))?
            .ok_or(Error::UnknownCommitment)?;
        let merkle_path = tree
            .witness_at_checkpoint_id(position, &0)
            .map_err(|e| Error::Stark(format!("Orchard witness lookup failed: {e:?}")))?
            .ok_or(Error::UnknownCommitment)?;

        let mut builder = Builder::new(
            BundleType::Transactional {
                flags: Flags::OUTPUTS_DISABLED,
                bundle_required: false,
            },
            root.into(),
        );
        builder
            .add_spend(fvk, note, merkle_path.into())
            .map_err(|e| Error::Stark(format!("Orchard spend build failed: {e:?}")))?;
        let (unauthorized, _) = builder
            .build(&mut rng)
            .map_err(|e| Error::Stark(format!("Orchard spend bundle build failed: {e:?}")))?
            .ok_or(Error::InvalidProof)?;
        let sighash = orchard_sighash(unauthorized.commitment().into(), b"thornado:withdraw");
        let proven = unauthorized
            .create_proof(&pk, &mut rng)
            .map_err(|e| Error::Stark(format!("Orchard spend proof failed: {e:?}")))?;
        proven
            .apply_signatures(&mut rng, sighash, &[SpendAuthorizingKey::from(&sk)])
            .map_err(|e| Error::Stark(format!("Orchard spend signature failed: {e:?}")))?
    };
    verify_bundle(&spend_bundle, &vk, b"thornado:withdraw")?;
    Ok(spend_bundle.authorization().proof().as_ref().to_vec())
}

fn orchard_path(
    leaves: &[String],
    commitment: &str,
) -> Result<(
    MerkleHashOrchard,
    incrementalmerkletree::MerklePath<MerkleHashOrchard, 32>,
)> {
    let mut tree: ShardTree<MemoryShardStore<MerkleHashOrchard, u32>, 32, 16> =
        ShardTree::new(MemoryShardStore::empty(), 100);
    let mut found = false;
    for leaf in leaves {
        let hash = parse_merkle_hash(leaf)?;
        tree.append(
            hash,
            Retention::Checkpoint {
                id: 0,
                marking: Marking::Marked,
            },
        )
        .map_err(|e| Error::Stark(format!("Orchard tree append failed: {e:?}")))?;
        if leaf == commitment {
            found = true;
        }
    }
    if !found {
        return Err(Error::UnknownCommitment);
    }
    let root = tree
        .root_at_checkpoint_id(&0)
        .map_err(|e| Error::Stark(format!("Orchard root lookup failed: {e:?}")))?
        .ok_or(Error::UnknownMerkleRoot)?;
    let position = leaves
        .iter()
        .position(|leaf| leaf == commitment)
        .ok_or(Error::UnknownCommitment)?;
    let merkle_path = tree
        .witness_at_checkpoint_id((position as u64).into(), &0)
        .map_err(|e| Error::Stark(format!("Orchard witness lookup failed: {e:?}")))?
        .ok_or(Error::UnknownCommitment)?;
    Ok((root, merkle_path))
}

fn verify_bundle(
    bundle: &Bundle<Authorized, i64>,
    vk: &VerifyingKey,
    context: &[u8],
) -> Result<()> {
    bundle
        .verify_proof(vk)
        .map_err(|e| Error::Stark(format!("Orchard proof verification failed: {e:?}")))?;
    let sighash = orchard_sighash(bundle.commitment().into(), context);
    for action in bundle.actions() {
        action
            .rk()
            .verify(&sighash, action.authorization())
            .map_err(|_| Error::InvalidProof)?;
    }
    bundle
        .binding_validating_key()
        .verify(&sighash, bundle.authorization().binding_signature())
        .map_err(|_| Error::InvalidProof)
}

fn bundle_to_payload(
    bundle: &Bundle<Authorized, i64>,
    public_context: &[u8],
) -> OrchardWithdrawalProof {
    OrchardWithdrawalProof {
        proof_hex: hex::encode(bundle.authorization().proof().as_ref()),
        binding_signature_hex: hex::encode(<[u8; 64]>::from(
            bundle.authorization().binding_signature(),
        )),
        anchor_hex: hex::encode(bundle.anchor().to_bytes()),
        public_context_hex: hex::encode(public_context),
        value_balance: *bundle.value_balance(),
        actions: bundle.actions().iter().map(action_to_payload).collect(),
    }
}

fn payload_to_bundle(payload: &OrchardWithdrawalProof) -> Result<Bundle<Authorized, i64>> {
    let mut actions = Vec::with_capacity(payload.actions.len());
    for action in &payload.actions {
        actions.push(payload_to_action(action)?);
    }
    let actions = NonEmpty::from_vec(actions).ok_or(Error::InvalidProof)?;
    let anchor = Anchor::from_bytes(parse_hex_32(&payload.anchor_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let proof = Proof::new(hex::decode(&payload.proof_hex).map_err(|_| Error::InvalidProof)?);
    let binding_signature =
        redpallas::Signature::<Binding>::from(parse_hex_64(&payload.binding_signature_hex)?);
    Ok(Bundle::from_parts(
        actions,
        Flags::OUTPUTS_DISABLED,
        payload.value_balance,
        anchor,
        Authorized::from_parts(proof, binding_signature),
    ))
}

fn action_to_public_payload<A>(action: &Action<A>) -> OrchardActionPayload {
    OrchardActionPayload {
        nullifier_hex: hex::encode(action.nullifier().to_bytes()),
        rk_hex: hex::encode(<[u8; 32]>::from(action.rk().clone())),
        cmx_hex: hex::encode(action.cmx().to_bytes()),
        cv_net_hex: hex::encode(action.cv_net().to_bytes()),
        epk_hex: hex::encode(action.encrypted_note().epk_bytes),
        enc_ciphertext_hex: hex::encode(action.encrypted_note().enc_ciphertext),
        out_ciphertext_hex: hex::encode(action.encrypted_note().out_ciphertext),
        spend_auth_sig_hex: String::new(),
    }
}

fn action_to_payload(action: &Action<redpallas::Signature<SpendAuth>>) -> OrchardActionPayload {
    let mut payload = action_to_public_payload(action);
    payload.spend_auth_sig_hex = hex::encode(<[u8; 64]>::from(action.authorization()));
    payload
}

fn payload_to_output_action(payload: &OrchardActionPayload) -> Result<Action<()>> {
    let nf = Nullifier::from_bytes(&parse_hex_32(&payload.nullifier_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let rk = redpallas::VerificationKey::<SpendAuth>::try_from(parse_hex_32(&payload.rk_hex)?)
        .map_err(|_| Error::InvalidProof)?;
    let cmx = ExtractedNoteCommitment::from_bytes(&parse_hex_32(&payload.cmx_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let cv_net = ValueCommitment::from_bytes(&parse_hex_32(&payload.cv_net_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let encrypted_note = TransmittedNoteCiphertext {
        epk_bytes: parse_hex_32(&payload.epk_hex)?,
        enc_ciphertext: parse_hex_array::<580>(&payload.enc_ciphertext_hex)?,
        out_ciphertext: parse_hex_array::<80>(&payload.out_ciphertext_hex)?,
    };
    Action::from_parts(nf, rk, cmx, encrypted_note, cv_net, ()).ok_or(Error::InvalidProof)
}

fn payload_to_action(
    payload: &OrchardActionPayload,
) -> Result<Action<redpallas::Signature<SpendAuth>>> {
    let nf = Nullifier::from_bytes(&parse_hex_32(&payload.nullifier_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let rk = redpallas::VerificationKey::<SpendAuth>::try_from(parse_hex_32(&payload.rk_hex)?)
        .map_err(|_| Error::InvalidProof)?;
    let cmx = ExtractedNoteCommitment::from_bytes(&parse_hex_32(&payload.cmx_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let cv_net = ValueCommitment::from_bytes(&parse_hex_32(&payload.cv_net_hex)?)
        .into_option()
        .ok_or(Error::InvalidProof)?;
    let encrypted_note = TransmittedNoteCiphertext {
        epk_bytes: parse_hex_32(&payload.epk_hex)?,
        enc_ciphertext: parse_hex_array::<580>(&payload.enc_ciphertext_hex)?,
        out_ciphertext: parse_hex_array::<80>(&payload.out_ciphertext_hex)?,
    };
    let authorization =
        redpallas::Signature::<SpendAuth>::from(parse_hex_64(&payload.spend_auth_sig_hex)?);
    Action::from_parts(nf, rk, cmx, encrypted_note, cv_net, authorization)
        .ok_or(Error::InvalidProof)
}

fn parse_merkle_hash(value: &str) -> Result<MerkleHashOrchard> {
    MerkleHashOrchard::from_bytes(&parse_hex_32(value)?)
        .into_option()
        .ok_or(Error::InvalidProof)
}

fn parse_hex_32(value: &str) -> Result<[u8; 32]> {
    parse_hex_array(value)
}

fn parse_hex_64(value: &str) -> Result<[u8; 64]> {
    parse_hex_array(value)
}

fn parse_hex_array<const N: usize>(value: &str) -> Result<[u8; N]> {
    let bytes = hex::decode(value).map_err(|_| Error::InvalidProof)?;
    bytes.try_into().map_err(|_| Error::InvalidProof)
}

fn orchard_sighash(bundle_commitment: [u8; 32], context: &[u8]) -> [u8; 32] {
    let digest = Sha256::digest(
        [
            b"thornado-orchard-v1".as_slice(),
            context,
            &bundle_commitment,
        ]
        .concat(),
    );
    digest.into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn orchard_spend_bundle_proves_and_verifies() {
        let proof = prove_orchard_spend_smoke("orchard-test-seed", 100_000).unwrap();
        assert!(!proof.is_empty());
    }

    #[test]
    fn orchard_keys_are_deterministic_and_addresses_do_not_expose_seed() {
        let a = raw_address_from_seed("client-seed").unwrap();
        let b = raw_address_from_seed("client-seed").unwrap();
        let c = raw_address_from_seed("other-seed").unwrap();
        assert_eq!(a, b);
        assert_ne!(a, c);
        assert!(!hex::encode(a).contains("client-seed"));
    }
}
