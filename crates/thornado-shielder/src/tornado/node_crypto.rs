//! Production-accurate Pedersen / MiMC / Merkle (pure Rust, circomlib-compatible).

use ark_bn254::Fr;

use super::pedersen;
use crate::Result;

pub fn note_commitment(nullifier: Fr, secret: Fr) -> Result<Fr> {
    pedersen::note_commitment(nullifier, secret).ok_or(crate::Error::InvalidProof)
}

pub fn nullifier_hash(nullifier: Fr) -> Result<Fr> {
    pedersen::nullifier_hash(nullifier).ok_or(crate::Error::InvalidProof)
}
