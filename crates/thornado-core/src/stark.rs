#![allow(dead_code)]

use crate::{Error, Result, WithdrawalPublicInputs};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use winterfell::crypto::{hashers::Blake3_256, DefaultRandomCoin, MerkleTree};
use winterfell::math::{fields::f128::BaseElement, FieldElement, StarkField, ToElements};
use winterfell::matrix::ColMatrix;
use winterfell::{
    Air, AirContext, Assertion, AuxRandElements, BatchingMethod, CompositionPoly,
    CompositionPolyTrace, ConstraintCompositionCoefficients, DefaultConstraintCommitment,
    DefaultConstraintEvaluator, DefaultTraceLde, EvaluationFrame, FieldExtension, PartitionOptions,
    ProofOptions, Prover, StarkDomain, TraceInfo, TracePolyTable, TraceTable,
    TransitionConstraintDegree,
};

pub const STARK_MERKLE_DEPTH: usize = 7;
const TRACE_WIDTH: usize = 7;
const TRACE_LENGTH: usize = STARK_MERKLE_DEPTH + 1;

const C_HASH_1: u128 = 17;
const C_HASH_2: u128 = 29;
const C_HASH_3: u128 = 41;
const C_HASH_4: u128 = 53;

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
        _proof: &crate::WithdrawalProof,
        _public: &WithdrawalPublicInputs,
    ) -> Result<()> {
        Err(Error::Stark(
            "Winterfell withdrawal AIR is not witness hiding; backend disabled".to_string(),
        ))
    }

    fn reveals_commitment(&self) -> bool {
        false
    }
}

#[derive(Debug, Clone)]
struct StarkPublicInputs {
    root: BaseElement,
    nullifier_hash: BaseElement,
    denomination: BaseElement,
    recipient: BaseElement,
    relayer: BaseElement,
    fee: BaseElement,
    refund: BaseElement,
}

impl ToElements<BaseElement> for StarkPublicInputs {
    fn to_elements(&self) -> Vec<BaseElement> {
        vec![
            self.root,
            self.nullifier_hash,
            self.denomination,
            self.recipient,
            self.relayer,
            self.fee,
            self.refund,
        ]
    }
}

struct WithdrawalAir {
    context: AirContext<BaseElement>,
    public: StarkPublicInputs,
}

impl Air for WithdrawalAir {
    type BaseField = BaseElement;
    type PublicInputs = StarkPublicInputs;

    fn new(trace_info: TraceInfo, public: StarkPublicInputs, options: ProofOptions) -> Self {
        assert_eq!(TRACE_WIDTH, trace_info.width());
        let degrees = vec![
            TransitionConstraintDegree::with_cycles(1, vec![TRACE_LENGTH]),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(1),
            TransitionConstraintDegree::new(3),
        ];
        Self {
            context: AirContext::new(trace_info, degrees, 3, options),
            public,
        }
    }

    fn evaluate_transition<E: FieldElement + From<Self::BaseField>>(
        &self,
        frame: &EvaluationFrame<E>,
        periodic_values: &[E],
        result: &mut [E],
    ) {
        let current = frame.current();
        let next = frame.next();
        let first_step = periodic_values[0];

        let acc = current[0];
        let sibling = current[1];
        let index = current[2];
        let nullifier = current[3];
        let secret = current[4];
        let denomination = current[5];
        let commitment = current[6];

        result[0] = first_step * (acc - commitment);
        result[1] = commitment - algebraic_hash3_e(nullifier, secret, denomination);
        result[2] = E::from(self.public.nullifier_hash) - algebraic_hash1_e(nullifier);
        result[3] = next[3] - nullifier;
        result[4] = next[4] - secret;
        result[5] = next[5] - denomination;
        result[6] = next[6] - commitment;

        let left_hash = algebraic_hash2_e(acc, sibling);
        let right_hash = algebraic_hash2_e(sibling, acc);
        let selected_hash = left_hash + index * (right_hash - left_hash);
        result[7] = index * (index - E::ONE);
        result[8] = next[0] - selected_hash;
    }

    fn get_assertions(&self) -> Vec<Assertion<Self::BaseField>> {
        vec![
            Assertion::single(0, TRACE_LENGTH - 1, self.public.root),
            Assertion::single(2, TRACE_LENGTH - 1, BaseElement::ZERO),
            Assertion::single(5, 0, self.public.denomination),
        ]
    }

    fn get_periodic_column_values(&self) -> Vec<Vec<Self::BaseField>> {
        let mut first_step = vec![BaseElement::ZERO; TRACE_LENGTH];
        first_step[0] = BaseElement::ONE;
        vec![first_step]
    }

    fn context(&self) -> &AirContext<Self::BaseField> {
        &self.context
    }
}

struct WithdrawalProver {
    options: ProofOptions,
    public: StarkPublicInputs,
}

impl WithdrawalProver {
    fn new(public: StarkPublicInputs) -> Self {
        Self {
            options: ProofOptions::new(
                32,
                8,
                0,
                FieldExtension::None,
                8,
                31,
                BatchingMethod::Linear,
                BatchingMethod::Linear,
            ),
            public,
        }
    }
}

impl Prover for WithdrawalProver {
    type BaseField = BaseElement;
    type Air = WithdrawalAir;
    type Trace = TraceTable<Self::BaseField>;
    type HashFn = Blake3_256<Self::BaseField>;
    type VC = MerkleTree<Self::HashFn>;
    type RandomCoin = DefaultRandomCoin<Self::HashFn>;
    type TraceLde<E: FieldElement<BaseField = Self::BaseField>> =
        DefaultTraceLde<E, Self::HashFn, Self::VC>;
    type ConstraintCommitment<E: FieldElement<BaseField = Self::BaseField>> =
        DefaultConstraintCommitment<E, Self::HashFn, Self::VC>;
    type ConstraintEvaluator<'a, E: FieldElement<BaseField = Self::BaseField>> =
        DefaultConstraintEvaluator<'a, Self::Air, E>;

    fn get_pub_inputs(&self, trace: &Self::Trace) -> StarkPublicInputs {
        let mut public = self.public.clone();
        public.root = trace.get(0, TRACE_LENGTH - 1);
        public.denomination = trace.get(5, 0);
        public
    }

    fn options(&self) -> &ProofOptions {
        &self.options
    }

    fn new_trace_lde<E: FieldElement<BaseField = Self::BaseField>>(
        &self,
        trace_info: &TraceInfo,
        main_trace: &ColMatrix<Self::BaseField>,
        domain: &StarkDomain<Self::BaseField>,
        partition_option: PartitionOptions,
    ) -> (Self::TraceLde<E>, TracePolyTable<E>) {
        DefaultTraceLde::new(trace_info, main_trace, domain, partition_option)
    }

    fn build_constraint_commitment<E: FieldElement<BaseField = Self::BaseField>>(
        &self,
        composition_poly_trace: CompositionPolyTrace<E>,
        num_constraint_composition_columns: usize,
        domain: &StarkDomain<Self::BaseField>,
        partition_options: PartitionOptions,
    ) -> (Self::ConstraintCommitment<E>, CompositionPoly<E>) {
        DefaultConstraintCommitment::new(
            composition_poly_trace,
            num_constraint_composition_columns,
            domain,
            partition_options,
        )
    }

    fn new_evaluator<'a, E: FieldElement<BaseField = Self::BaseField>>(
        &self,
        air: &'a Self::Air,
        aux_rand_elements: Option<AuxRandElements<E>>,
        composition_coefficients: ConstraintCompositionCoefficients<E>,
    ) -> Self::ConstraintEvaluator<'a, E> {
        DefaultConstraintEvaluator::new(air, aux_rand_elements, composition_coefficients)
    }
}

pub fn prove_stark_withdrawal(
    _nullifier: &str,
    _secret: &str,
    _denomination_sats: u64,
    _path: &StarkMerklePath,
    _public: &WithdrawalPublicInputs,
) -> Result<TornadoStarkProof> {
    Err(Error::Stark(
        "Winterfell withdrawal AIR is not witness hiding; backend disabled".to_string(),
    ))
}

pub fn verify_stark_withdrawal(
    _stark: &TornadoStarkProof,
    _public: &WithdrawalPublicInputs,
) -> Result<()> {
    Err(Error::Stark(
        "Winterfell withdrawal AIR is not witness hiding; backend disabled".to_string(),
    ))
}

pub fn stark_merkle_path(leaves: &[String], index: usize) -> Result<StarkMerklePath> {
    if index >= leaves.len() || leaves.len() > (1 << STARK_MERKLE_DEPTH) {
        return Err(Error::UnknownCommitment);
    }

    let mut level: Vec<BaseElement> = padded_leaves(leaves)?;
    let mut node_index = index;
    let mut siblings = Vec::with_capacity(STARK_MERKLE_DEPTH);
    let mut indices = Vec::with_capacity(STARK_MERKLE_DEPTH);
    for _ in 0..STARK_MERKLE_DEPTH {
        let sibling_index = node_index ^ 1;
        siblings.push(field_to_string(level[sibling_index]));
        indices.push((node_index & 1) as u8);

        level = level
            .chunks_exact(2)
            .map(|pair| algebraic_hash2_f(pair[0], pair[1]))
            .collect();
        node_index /= 2;
    }

    Ok(StarkMerklePath { siblings, indices })
}

pub fn fixed_depth_merkle_root(leaves: &[String]) -> String {
    match fixed_depth_merkle_root_checked(leaves) {
        Ok(root) => root,
        Err(_) => field_to_string(BaseElement::ZERO),
    }
}

fn fixed_depth_merkle_root_checked(leaves: &[String]) -> Result<String> {
    let mut level = padded_leaves(leaves)?;
    for _ in 0..STARK_MERKLE_DEPTH {
        level = level
            .chunks_exact(2)
            .map(|pair| algebraic_hash2_f(pair[0], pair[1]))
            .collect();
    }
    Ok(field_to_string(level[0]))
}

pub fn algebraic_hash1(value: String) -> String {
    let value = parse_field(&value).unwrap_or(BaseElement::ZERO);
    field_to_string(algebraic_hash1_f(value))
}

pub fn algebraic_hash3(left: String, right: String, third: String) -> String {
    let left = parse_field(&left).unwrap_or(BaseElement::ZERO);
    let right = parse_field(&right).unwrap_or(BaseElement::ZERO);
    let third = parse_field(&third).unwrap_or(BaseElement::ZERO);
    field_to_string(algebraic_hash3_f(left, right, third))
}

pub fn stark_field_from_bytes(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    let mut reduced = [0_u8; 16];
    reduced.copy_from_slice(&digest[..16]);
    field_to_string(BaseElement::new(u128::from_be_bytes(reduced)))
}

pub fn stark_field_from_u64(value: u64) -> String {
    field_to_string(BaseElement::new(value as u128))
}

pub fn stark_field_from_string(value: &str) -> String {
    parse_field(value)
        .map(field_to_string)
        .unwrap_or_else(|_| stark_field_from_bytes(value.as_bytes()))
}

fn build_trace(
    nullifier: &str,
    secret: &str,
    denomination_sats: u64,
    path: &StarkMerklePath,
) -> Result<TraceTable<BaseElement>> {
    if path.siblings.len() != STARK_MERKLE_DEPTH || path.indices.len() != STARK_MERKLE_DEPTH {
        return Err(Error::InvalidProof);
    }
    let nullifier = parse_field(nullifier)?;
    let secret = parse_field(secret)?;
    let denomination = BaseElement::new(denomination_sats as u128);
    let commitment = algebraic_hash3_f(nullifier, secret, denomination);
    let mut acc = vec![BaseElement::ZERO; TRACE_LENGTH];
    let mut siblings = vec![BaseElement::ZERO; TRACE_LENGTH];
    let mut indices = vec![BaseElement::ZERO; TRACE_LENGTH];
    let nullifiers = vec![nullifier; TRACE_LENGTH];
    let secrets = vec![secret; TRACE_LENGTH];
    let denominations = vec![denomination; TRACE_LENGTH];
    let commitments = vec![commitment; TRACE_LENGTH];

    acc[0] = commitment;
    for step in 0..STARK_MERKLE_DEPTH {
        siblings[step] = parse_field(&path.siblings[step])?;
        indices[step] = BaseElement::new(path.indices[step] as u128);
        acc[step + 1] = if path.indices[step] == 0 {
            algebraic_hash2_f(acc[step], siblings[step])
        } else {
            algebraic_hash2_f(siblings[step], acc[step])
        };
    }
    siblings[STARK_MERKLE_DEPTH] = BaseElement::ZERO;
    indices[STARK_MERKLE_DEPTH] = BaseElement::ZERO;

    Ok(TraceTable::init(vec![
        acc,
        siblings,
        indices,
        nullifiers,
        secrets,
        denominations,
        commitments,
    ]))
}

fn public_inputs(public: &WithdrawalPublicInputs) -> Result<StarkPublicInputs> {
    Ok(StarkPublicInputs {
        root: parse_field(&public.merkle_root)?,
        nullifier_hash: parse_field(&public.nullifier_hash)?,
        denomination: BaseElement::new(public.denomination_sats as u128),
        recipient: parse_optional_public_field(public.recipient_field.as_deref())?,
        relayer: parse_optional_public_field(public.relayer_field.as_deref())?,
        fee: BaseElement::new(public.fee_sats as u128),
        refund: parse_optional_public_field(public.refund_field.as_deref())?,
    })
}

fn parse_optional_public_field(value: Option<&str>) -> Result<BaseElement> {
    value.map(parse_field).unwrap_or(Ok(BaseElement::ZERO))
}

fn padded_leaves(leaves: &[String]) -> Result<Vec<BaseElement>> {
    if leaves.len() > (1 << STARK_MERKLE_DEPTH) {
        return Err(Error::InvalidProof);
    }
    let mut level = vec![BaseElement::ZERO; 1 << STARK_MERKLE_DEPTH];
    for (index, leaf) in leaves.iter().enumerate() {
        level[index] = parse_field(leaf)?;
    }
    Ok(level)
}

fn parse_field(value: &str) -> Result<BaseElement> {
    let value = value.strip_prefix("0x").unwrap_or(value);
    if value.is_empty() {
        return Err(Error::InvalidFieldElement);
    }
    if value.bytes().all(|byte| byte.is_ascii_digit()) {
        value
            .parse::<u128>()
            .map(BaseElement::new)
            .map_err(|_| Error::InvalidFieldElement)
    } else if value.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        let mut hex = value.to_string();
        if hex.len() % 2 == 1 {
            hex.insert(0, '0');
        }
        let bytes = hex::decode(hex).map_err(|_| Error::InvalidFieldElement)?;
        let mut reduced = [0_u8; 16];
        let start = bytes.len().saturating_sub(16);
        reduced[(16 - (bytes.len() - start))..].copy_from_slice(&bytes[start..]);
        Ok(BaseElement::new(u128::from_be_bytes(reduced)))
    } else {
        Err(Error::InvalidFieldElement)
    }
}

fn field_to_string(value: BaseElement) -> String {
    value.as_int().to_string()
}

fn algebraic_hash1_f(value: BaseElement) -> BaseElement {
    value.exp(5_u32.into()) + BaseElement::new(C_HASH_1)
}

fn algebraic_hash2_f(left: BaseElement, right: BaseElement) -> BaseElement {
    left.exp(3_u32.into())
        + (right.exp(3_u32.into()) * BaseElement::new(C_HASH_2))
        + BaseElement::new(C_HASH_3)
}

fn algebraic_hash3_f(left: BaseElement, right: BaseElement, third: BaseElement) -> BaseElement {
    algebraic_hash2_f(algebraic_hash2_f(left, right), third) + BaseElement::new(C_HASH_4)
}

fn algebraic_hash1_e<E: FieldElement + From<BaseElement>>(value: E) -> E {
    value.exp(5_u32.into()) + E::from(BaseElement::new(C_HASH_1))
}

fn algebraic_hash2_e<E: FieldElement + From<BaseElement>>(left: E, right: E) -> E {
    left.exp(3_u32.into())
        + (right.exp(3_u32.into()) * E::from(BaseElement::new(C_HASH_2)))
        + E::from(BaseElement::new(C_HASH_3))
}

fn algebraic_hash3_e<E: FieldElement + From<BaseElement>>(left: E, right: E, third: E) -> E {
    algebraic_hash2_e(algebraic_hash2_e(left, right), third) + E::from(BaseElement::new(C_HASH_4))
}
