use crate::{Error, Result, WithdrawalPublicInputs};
use ark_bn254::{Bn254, Fr};
use ark_ff::{Field, PrimeField, Zero};
use ark_groth16::{prepare_verifying_key, Groth16, Proof, ProvingKey};
use ark_relations::lc;
use ark_relations::r1cs::{
    ConstraintSynthesizer, ConstraintSystemRef, LinearCombination, SynthesisError, Variable,
};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::str::FromStr;
use std::sync::LazyLock;
use tiny_keccak::{Hasher, Keccak};

pub const STARK_MERKLE_DEPTH: usize = 7;
const MIMC_ROUNDS: usize = 220;

static GROTH16_PROVING_KEY: LazyLock<ProvingKey<Bn254>> = LazyLock::new(|| {
    let mut rng = rand_core::OsRng;
    Groth16::<Bn254>::generate_random_parameters_with_reduction(
        WithdrawalCircuit::blank(),
        &mut rng,
    )
    .expect("Groth16 setup for withdrawal circuit")
});

static MIMC_ROUND_CONSTANTS: LazyLock<Vec<Fr>> = LazyLock::new(mimc_round_constants);

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TornadoStarkProof {
    pub proof_hex: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct StarkMerklePath {
    pub siblings: Vec<String>,
    pub indices: Vec<u8>,
}

#[derive(Debug, Default, Clone)]
pub struct StarkProofVerifier;

impl crate::ProofVerifier for StarkProofVerifier {
    fn verify_withdrawal(
        &self,
        proof: &crate::WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()> {
        #[cfg(feature = "orchard-zcash")]
        if let Some(orchard) = proof.orchard.as_ref() {
            if orchard.anchor_hex != public.merkle_root {
                return Err(Error::InvalidProof);
            }
            if orchard.value_balance != public.denomination_sats as i64 {
                return Err(Error::InvalidProof);
            }
            let public_context = crate::orchard_public_context(public);
            return crate::orchard::verify_orchard_withdrawal(orchard, &public_context);
        }
        let stark = proof.stark.as_ref().ok_or(Error::InvalidProof)?;
        verify_stark_withdrawal(stark, public)
    }

    fn reveals_commitment(&self) -> bool {
        false
    }
}

#[derive(Debug, Clone)]
struct WithdrawalWitness {
    nullifier: Fr,
    secret: Fr,
    owner_secret: Fr,
    denomination: Fr,
    siblings: Vec<Fr>,
    indices: Vec<Fr>,
}

#[derive(Debug, Clone)]
struct WithdrawalCircuit {
    public: Groth16PublicInputs,
    witness: WithdrawalWitness,
}

#[derive(Debug, Clone)]
struct Groth16PublicInputs {
    root: Fr,
    nullifier_hash: Fr,
    owner_pubkey: Fr,
    denomination: Fr,
    recipient: Fr,
    relayer: Fr,
    fee: Fr,
    refund: Fr,
}

#[derive(Debug, Clone)]
struct AllocatedValue {
    variable: Variable,
    value: Fr,
}

impl WithdrawalCircuit {
    fn blank() -> Self {
        Self {
            public: Groth16PublicInputs {
                root: Fr::zero(),
                nullifier_hash: Fr::zero(),
                owner_pubkey: Fr::zero(),
                denomination: Fr::zero(),
                recipient: Fr::zero(),
                relayer: Fr::zero(),
                fee: Fr::zero(),
                refund: Fr::zero(),
            },
            witness: WithdrawalWitness {
                nullifier: Fr::zero(),
                secret: Fr::zero(),
                owner_secret: Fr::zero(),
                denomination: Fr::zero(),
                siblings: vec![Fr::zero(); STARK_MERKLE_DEPTH],
                indices: vec![Fr::zero(); STARK_MERKLE_DEPTH],
            },
        }
    }
}

impl Groth16PublicInputs {
    fn from_public(public: &WithdrawalPublicInputs) -> Result<Self> {
        let expected_recipient = stark_field_from_bytes(public.recipient.as_bytes());
        if public.recipient_field.as_deref() != Some(expected_recipient.as_str()) {
            return Err(Error::InvalidProof);
        }
        Ok(Self {
            root: parse_fr(&public.merkle_root)?,
            nullifier_hash: parse_fr(&public.nullifier_hash)?,
            owner_pubkey: parse_optional_fr(Some(public.owner_pubkey.as_str()))?,
            denomination: fr_from_u64(public.denomination_sats),
            recipient: parse_optional_fr(public.recipient_field.as_deref())?,
            relayer: parse_optional_fr(public.relayer_field.as_deref())?,
            fee: fr_from_u64(public.fee_sats),
            refund: parse_optional_fr(public.refund_field.as_deref())?,
        })
    }

    fn to_vec(&self) -> Vec<Fr> {
        vec![
            self.root,
            self.nullifier_hash,
            self.owner_pubkey,
            self.denomination,
            self.recipient,
            self.relayer,
            self.fee,
            self.refund,
        ]
    }
}

impl ConstraintSynthesizer<Fr> for WithdrawalCircuit {
    fn generate_constraints(
        self,
        cs: ConstraintSystemRef<Fr>,
    ) -> std::result::Result<(), SynthesisError> {
        let root = alloc_input(&cs, self.public.root)?;
        let nullifier_hash = alloc_input(&cs, self.public.nullifier_hash)?;
        let owner_pubkey = alloc_input(&cs, self.public.owner_pubkey)?;
        let denomination_public = alloc_input(&cs, self.public.denomination)?;
        let recipient = alloc_input(&cs, self.public.recipient)?;
        let relayer = alloc_input(&cs, self.public.relayer)?;
        let fee = alloc_input(&cs, self.public.fee)?;
        let refund = alloc_input(&cs, self.public.refund)?;

        let nullifier = alloc_witness(&cs, self.witness.nullifier)?;
        let secret = alloc_witness(&cs, self.witness.secret)?;
        let owner_secret = alloc_witness(&cs, self.witness.owner_secret)?;
        let denomination = alloc_witness(&cs, self.witness.denomination)?;
        enforce_equal(&cs, denomination.variable, denomination_public.variable)?;

        let computed_owner_pubkey = mimc_sponge_gadget(&cs, &[owner_secret.clone()])?;
        enforce_equal(&cs, computed_owner_pubkey.variable, owner_pubkey.variable)?;

        let commitment = mimc_sponge_gadget(
            &cs,
            &[nullifier.clone(), secret, denomination, owner_pubkey],
        )?;
        let computed_nullifier_hash = mimc_sponge_gadget(&cs, &[nullifier, owner_secret])?;
        enforce_equal(
            &cs,
            computed_nullifier_hash.variable,
            nullifier_hash.variable,
        )?;

        let mut acc = commitment;
        for depth in 0..STARK_MERKLE_DEPTH {
            let sibling = alloc_witness(&cs, self.witness.siblings[depth])?;
            let index = alloc_witness(&cs, self.witness.indices[depth])?;
            enforce_boolean(&cs, &index)?;

            let left = select_by_bit(&cs, &acc, &sibling, &index)?;
            let right = select_by_bit(&cs, &sibling, &acc, &index)?;
            acc = mimc_sponge_gadget(&cs, &[left, right])?;
        }
        enforce_equal(&cs, acc.variable, root.variable)?;

        bind_public_field(&cs, &recipient)?;
        bind_public_field(&cs, &relayer)?;
        bind_public_field(&cs, &fee)?;
        bind_public_field(&cs, &refund)?;
        Ok(())
    }
}

pub fn prove_stark_withdrawal(
    nullifier: &str,
    secret: &str,
    owner_secret: &str,
    denomination_sats: u64,
    path: &StarkMerklePath,
    public: &WithdrawalPublicInputs,
) -> Result<TornadoStarkProof> {
    if path.siblings.len() != STARK_MERKLE_DEPTH || path.indices.len() != STARK_MERKLE_DEPTH {
        return Err(Error::InvalidProof);
    }

    let witness = WithdrawalWitness {
        nullifier: parse_fr(&stark_field_from_string(nullifier))?,
        secret: parse_fr(&stark_field_from_string(secret))?,
        owner_secret: parse_fr(&stark_field_from_string(owner_secret))?,
        denomination: fr_from_u64(denomination_sats),
        siblings: path
            .siblings
            .iter()
            .map(|value| parse_fr(value))
            .collect::<Result<Vec<_>>>()?,
        indices: path
            .indices
            .iter()
            .map(|index| fr_from_u64(*index as u64))
            .collect(),
    };
    let circuit = WithdrawalCircuit {
        public: Groth16PublicInputs::from_public(public)?,
        witness,
    };
    let mut rng = rand_core::OsRng;
    let proof = Groth16::<Bn254>::create_random_proof_with_reduction(
        circuit,
        &GROTH16_PROVING_KEY,
        &mut rng,
    )
    .map_err(|e| Error::Stark(format!("Groth16 proving failed: {e}")))?;

    let mut proof_bytes = Vec::new();
    proof
        .serialize_compressed(&mut proof_bytes)
        .map_err(|e| Error::Stark(format!("Groth16 proof serialization failed: {e}")))?;
    Ok(TornadoStarkProof {
        proof_hex: hex::encode(proof_bytes),
    })
}

pub fn verify_stark_withdrawal(
    stark: &TornadoStarkProof,
    public: &WithdrawalPublicInputs,
) -> Result<()> {
    let proof_bytes = hex::decode(&stark.proof_hex)
        .map_err(|e| Error::Stark(format!("invalid proof hex: {e}")))?;
    let proof = Proof::<Bn254>::deserialize_compressed(&*proof_bytes)
        .map_err(|e| Error::Stark(format!("invalid Groth16 proof bytes: {e}")))?;
    let public_inputs = Groth16PublicInputs::from_public(public)?.to_vec();
    let pvk = prepare_verifying_key(&GROTH16_PROVING_KEY.vk);
    let verified = Groth16::<Bn254>::verify_proof(&pvk, &proof, &public_inputs)
        .map_err(|e| Error::Stark(format!("Groth16 verification failed: {e}")))?;
    if verified {
        Ok(())
    } else {
        Err(Error::InvalidProof)
    }
}

pub fn stark_merkle_path(leaves: &[String], index: usize) -> Result<StarkMerklePath> {
    if index >= leaves.len() || leaves.len() > (1 << STARK_MERKLE_DEPTH) {
        return Err(Error::UnknownCommitment);
    }

    let mut level = padded_fr_leaves(leaves)?;
    let mut node_index = index;
    let mut siblings = Vec::with_capacity(STARK_MERKLE_DEPTH);
    let mut indices = Vec::with_capacity(STARK_MERKLE_DEPTH);
    for _ in 0..STARK_MERKLE_DEPTH {
        let sibling_index = node_index ^ 1;
        siblings.push(fr_to_string(level[sibling_index]));
        indices.push((node_index & 1) as u8);

        level = level
            .chunks_exact(2)
            .map(|pair| mimc_sponge(&[pair[0], pair[1]]))
            .collect();
        node_index /= 2;
    }

    Ok(StarkMerklePath { siblings, indices })
}

pub fn fixed_depth_merkle_root(leaves: &[String]) -> String {
    match fixed_depth_merkle_root_checked(leaves) {
        Ok(root) => root,
        Err(_) => fr_to_string(Fr::zero()),
    }
}

pub fn algebraic_hash1(value: String) -> String {
    let value = parse_fr(&value).unwrap_or_else(|_| Fr::zero());
    fr_to_string(mimc_sponge(&[value]))
}

pub fn algebraic_hash_many(values: &[String]) -> String {
    let values = values
        .iter()
        .map(|value| parse_fr(value).unwrap_or_else(|_| Fr::zero()))
        .collect::<Vec<_>>();
    fr_to_string(mimc_sponge(&values))
}

pub fn stark_field_from_bytes(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    fr_to_string(Fr::from_be_bytes_mod_order(&digest))
}

pub fn stark_field_from_u64(value: u64) -> String {
    fr_to_string(fr_from_u64(value))
}

pub fn stark_field_from_string(value: &str) -> String {
    parse_fr(value)
        .map(fr_to_string)
        .unwrap_or_else(|_| stark_field_from_bytes(value.as_bytes()))
}

fn fixed_depth_merkle_root_checked(leaves: &[String]) -> Result<String> {
    let mut level = padded_fr_leaves(leaves)?;
    for _ in 0..STARK_MERKLE_DEPTH {
        level = level
            .chunks_exact(2)
            .map(|pair| mimc_sponge(&[pair[0], pair[1]]))
            .collect();
    }
    Ok(fr_to_string(level[0]))
}

fn padded_fr_leaves(leaves: &[String]) -> Result<Vec<Fr>> {
    if leaves.len() > (1 << STARK_MERKLE_DEPTH) {
        return Err(Error::InvalidProof);
    }
    let mut level = vec![Fr::zero(); 1 << STARK_MERKLE_DEPTH];
    for (index, leaf) in leaves.iter().enumerate() {
        level[index] = parse_fr(leaf)?;
    }
    Ok(level)
}

fn alloc_input(
    cs: &ConstraintSystemRef<Fr>,
    value: Fr,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    Ok(AllocatedValue {
        variable: cs.new_input_variable(|| Ok(value))?,
        value,
    })
}

fn alloc_witness(
    cs: &ConstraintSystemRef<Fr>,
    value: Fr,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    Ok(AllocatedValue {
        variable: cs.new_witness_variable(|| Ok(value))?,
        value,
    })
}

fn alloc_product(
    cs: &ConstraintSystemRef<Fr>,
    left: &AllocatedValue,
    right: &AllocatedValue,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    let value = left.value * right.value;
    let product = alloc_witness(cs, value)?;
    cs.enforce_constraint(
        lc!() + left.variable,
        lc!() + right.variable,
        lc!() + product.variable,
    )?;
    Ok(product)
}

fn enforce_boolean(
    cs: &ConstraintSystemRef<Fr>,
    bit: &AllocatedValue,
) -> std::result::Result<(), SynthesisError> {
    cs.enforce_constraint(
        lc!() + bit.variable,
        lc!() + bit.variable - Variable::One,
        lc!(),
    )
}

fn enforce_equal(
    cs: &ConstraintSystemRef<Fr>,
    left: Variable,
    right: Variable,
) -> std::result::Result<(), SynthesisError> {
    cs.enforce_constraint(lc!() + left - right, lc!() + Variable::One, lc!())
}

fn enforce_lc_equal(
    cs: &ConstraintSystemRef<Fr>,
    left: LinearCombination<Fr>,
    right: Variable,
) -> std::result::Result<(), SynthesisError> {
    cs.enforce_constraint(left - right, lc!() + Variable::One, lc!())
}

fn bind_public_field(
    cs: &ConstraintSystemRef<Fr>,
    value: &AllocatedValue,
) -> std::result::Result<(), SynthesisError> {
    let _square = alloc_product(cs, value, value)?;
    Ok(())
}

fn mimc_sponge_gadget(
    cs: &ConstraintSystemRef<Fr>,
    inputs: &[AllocatedValue],
) -> std::result::Result<AllocatedValue, SynthesisError> {
    let mut left = None;
    let mut right = alloc_witness(cs, Fr::zero())?;
    enforce_lc_equal(cs, lc!(), right.variable)?;
    for input in inputs {
        let next_left = if let Some(previous_left) = left {
            add_gadget(cs, &previous_left, input)?
        } else {
            input.clone()
        };
        let (new_left, new_right) = mimc_feistel_gadget(cs, &next_left, &right)?;
        left = Some(new_left);
        right = new_right;
    }
    left.ok_or(SynthesisError::AssignmentMissing)
}

fn mimc_feistel_gadget(
    cs: &ConstraintSystemRef<Fr>,
    left: &AllocatedValue,
    right: &AllocatedValue,
) -> std::result::Result<(AllocatedValue, AllocatedValue), SynthesisError> {
    let mut x_l = left.clone();
    let mut x_r = right.clone();
    for (round, constant) in MIMC_ROUND_CONSTANTS.iter().copied().enumerate() {
        let t = add_constant_gadget(cs, &x_l, constant)?;
        let t2 = alloc_product(cs, &t, &t)?;
        let t4 = alloc_product(cs, &t2, &t2)?;
        let t5 = alloc_product(cs, &t4, &t)?;
        if round < MIMC_ROUNDS - 1 {
            let next_l = add_gadget(cs, &x_r, &t5)?;
            x_r = x_l;
            x_l = next_l;
        } else {
            x_r = add_gadget(cs, &x_r, &t5)?;
        }
    }
    Ok((x_l, x_r))
}

fn select_by_bit(
    cs: &ConstraintSystemRef<Fr>,
    left: &AllocatedValue,
    right: &AllocatedValue,
    bit: &AllocatedValue,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    let diff_value = right.value - left.value;
    let selected_diff = alloc_witness(cs, bit.value * diff_value)?;
    cs.enforce_constraint(
        lc!() + bit.variable,
        lc!() + right.variable - left.variable,
        lc!() + selected_diff.variable,
    )?;

    let selected = alloc_witness(cs, left.value + selected_diff.value)?;
    enforce_lc_equal(
        cs,
        lc!() + left.variable + selected_diff.variable,
        selected.variable,
    )?;
    Ok(selected)
}

fn parse_optional_fr(value: Option<&str>) -> Result<Fr> {
    value.map(parse_fr).unwrap_or(Ok(Fr::zero()))
}

fn add_gadget(
    cs: &ConstraintSystemRef<Fr>,
    left: &AllocatedValue,
    right: &AllocatedValue,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    let out = alloc_witness(cs, left.value + right.value)?;
    enforce_lc_equal(cs, lc!() + left.variable + right.variable, out.variable)?;
    Ok(out)
}

fn add_constant_gadget(
    cs: &ConstraintSystemRef<Fr>,
    value: &AllocatedValue,
    constant: Fr,
) -> std::result::Result<AllocatedValue, SynthesisError> {
    let out = alloc_witness(cs, value.value + constant)?;
    enforce_lc_equal(
        cs,
        lc!() + value.variable + (constant, Variable::One),
        out.variable,
    )?;
    Ok(out)
}

fn parse_fr(value: &str) -> Result<Fr> {
    let has_hex_prefix = value.starts_with("0x");
    let value = value.strip_prefix("0x").unwrap_or(value);
    if value.is_empty() {
        return Err(Error::InvalidFieldElement);
    }
    if !has_hex_prefix && value.bytes().all(|byte| byte.is_ascii_digit()) {
        Fr::from_str(value).map_err(|_| Error::InvalidFieldElement)
    } else if value.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        let mut hex = value.to_string();
        if hex.len() % 2 == 1 {
            hex.insert(0, '0');
        }
        let bytes = hex::decode(hex).map_err(|_| Error::InvalidFieldElement)?;
        Ok(Fr::from_be_bytes_mod_order(&bytes))
    } else {
        Err(Error::InvalidFieldElement)
    }
}

fn fr_from_u64(value: u64) -> Fr {
    Fr::from(value)
}

fn fr_to_string(value: Fr) -> String {
    value.into_bigint().to_string()
}

fn mimc_sponge(inputs: &[Fr]) -> Fr {
    let mut left = Fr::zero();
    let mut right = Fr::zero();
    for input in inputs {
        left += input;
        (left, right) = mimc_feistel(left, right);
    }
    left
}

fn mimc_feistel(mut left: Fr, mut right: Fr) -> (Fr, Fr) {
    for (round, constant) in MIMC_ROUND_CONSTANTS.iter().copied().enumerate() {
        let t5 = (left + constant).pow([5]);
        if round < MIMC_ROUNDS - 1 {
            let next_left = right + t5;
            right = left;
            left = next_left;
        } else {
            right += t5;
        }
    }
    (left, right)
}

fn mimc_round_constants() -> Vec<Fr> {
    let mut seed = b"mimcsponge".to_vec();
    let mut constants = Vec::with_capacity(MIMC_ROUNDS);
    for round in 0..MIMC_ROUNDS {
        seed = keccak256(&seed).to_vec();
        if round == 0 || round == MIMC_ROUNDS - 1 {
            constants.push(Fr::zero());
        } else {
            constants.push(Fr::from_be_bytes_mod_order(&seed));
        }
    }
    constants
}

fn keccak256(bytes: &[u8]) -> [u8; 32] {
    let mut output = [0_u8; 32];
    let mut keccak = Keccak::v256();
    keccak.update(bytes);
    keccak.finalize(&mut output);
    output
}
