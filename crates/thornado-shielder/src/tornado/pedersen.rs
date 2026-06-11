//! Pedersen hash on Baby Jubjub (circomlib `pedersen_hash.js`).

use ark_bn254::Fr;
use ark_ff::{BigInteger, PrimeField};
use num_bigint::{BigInt, BigUint};
use num_traits::{Num, One, Zero};

use super::babyjub::{add_points, get_base_point, mul_base_point_escalar, zero_point};
use super::field::fr_to_hex;

const WINDOW_SIZE: usize = 4;
const N_WINDOWS_PER_SEGMENT: usize = 50;

fn buffer_to_bits(msg: &[u8]) -> Vec<bool> {
    let mut bits = Vec::with_capacity(msg.len() * 8);
    for byte in msg {
        for shift in 0..8 {
            bits.push((byte >> shift) & 1 == 1);
        }
    }
    bits
}

fn suborder() -> BigUint {
    BigUint::from_str_radix(
        "21888242871839275222246405745257275088614511777268538073601725287587578984328",
        10,
    )
    .expect("suborder")
        >> 3u32
}

fn pedersen_hash(msg: &[u8]) -> Option<[u8; 32]> {
    let bits = buffer_to_bits(msg);
    let bits_per_segment = WINDOW_SIZE * N_WINDOWS_PER_SEGMENT;
    let n_segments = (bits.len().saturating_sub(1)) / bits_per_segment + 1;
    let mut acc = zero_point();
    let suborder = suborder();

    for segment in 0..n_segments {
        let n_windows = if segment == n_segments - 1 {
            ((bits.len() - segment * bits_per_segment).saturating_sub(1)) / WINDOW_SIZE + 1
        } else {
            N_WINDOWS_PER_SEGMENT
        };
        let base = get_base_point(segment as u32)?;
        let mut escalar = BigInt::zero();
        let mut exp = BigInt::one();
        for window in 0..n_windows {
            let mut offset = segment * bits_per_segment + window * WINDOW_SIZE;
            let mut acc_scalar = BigInt::one();
            for bit in 0..(WINDOW_SIZE - 1) {
                if offset < bits.len() && bits[offset] {
                    acc_scalar += BigInt::one() << bit;
                }
                offset += 1;
            }
            if offset < bits.len() && bits[offset] {
                acc_scalar = -acc_scalar;
            }
            escalar += &acc_scalar * &exp;
            exp <<= WINDOW_SIZE + 1;
        }
        if escalar < BigInt::zero() {
            escalar += BigInt::from(suborder.clone());
        }
        let sub = BigInt::from(suborder.clone());
        let escalar = escalar % &sub;
        let escalar = escalar.to_biguint().unwrap_or_default();
        let point = mul_base_point_escalar(base, escalar);
        acc = add_points(acc, point);
    }

    Some(super::babyjub::pack_point(acc))
}

fn field_element_bits(value: Fr) -> [u8; 31] {
    let repr = value.into_bigint().to_bytes_le();
    let mut out = [0u8; 31];
    out[..repr.len().min(31)].copy_from_slice(&repr[..repr.len().min(31)]);
    out
}

pub fn note_commitment(nullifier: Fr, secret: Fr) -> Option<Fr> {
    let mut msg = Vec::with_capacity(62);
    msg.extend_from_slice(&field_element_bits(nullifier));
    msg.extend_from_slice(&field_element_bits(secret));
    let packed = pedersen_hash(&msg)?;
    super::babyjub::point_x_from_packed(packed)
}

pub fn nullifier_hash(nullifier: Fr) -> Option<Fr> {
    let msg = field_element_bits(nullifier);
    let packed = pedersen_hash(&msg)?;
    super::babyjub::point_x_from_packed(packed)
}

#[allow(dead_code)]
pub fn hash_to_hex(value: Fr) -> Option<String> {
    note_commitment(value, Fr::zero()).map(|_| fr_to_hex(value))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::field::{fr_from_hex, fr_to_decimal};

    #[test]
    fn matches_circomlib_vectors() {
        let vectors: serde_json::Value =
            serde_json::from_str(include_str!("../../testdata/tornado_vectors.json")).unwrap();
        for case in vectors["pedersen"].as_array().unwrap() {
            let nullifier = fr_from_hex(case["nullifier"].as_str().unwrap()).unwrap();
            let secret = fr_from_hex(case["secret"].as_str().unwrap()).unwrap();
            assert_eq!(
                fr_to_decimal(note_commitment(nullifier, secret).unwrap()),
                case["commitment"].as_str().unwrap()
            );
            assert_eq!(
                fr_to_decimal(nullifier_hash(nullifier).unwrap()),
                case["nullifierHash"].as_str().unwrap()
            );
        }
    }
}
