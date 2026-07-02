//! MiMC sponge (220 rounds, x^5) matching circomlibjs `mimcsponge.js`.

use std::sync::OnceLock;

use ark_bn254::Fr;
use ark_ff::{Field, Zero};
use sha3::{Digest, Keccak256};

use super::field::{fr_from_hex, fr_to_decimal};
use crate::Result;

const NROUNDS: usize = 220;
const SEED: &[u8] = b"mimcsponge";

fn keccak256(input: impl AsRef<[u8]>) -> [u8; 32] {
    Keccak256::digest(input.as_ref()).into()
}

fn round_constants() -> &'static [Fr; NROUNDS] {
    static CONSTANTS: OnceLock<[Fr; NROUNDS]> = OnceLock::new();
    CONSTANTS.get_or_init(|| {
        let mut cts = [Fr::zero(); NROUNDS];
        let mut digest = keccak256(SEED);
        for i in 1..NROUNDS {
            digest = keccak256(digest);
            cts[i] = fr_from_hex(&hex::encode(digest)).expect("round constant fits field");
        }
        cts[NROUNDS - 1] = Fr::zero();
        cts
    })
}

fn sponge_hash(xl_in: Fr, xr_in: Fr, key: Fr) -> (Fr, Fr) {
    let cts = round_constants();
    let mut xl = xl_in;
    let mut xr = xr_in;
    for i in 0..NROUNDS {
        let c = cts[i];
        let t = if i == 0 { xl + key } else { xl + key + c };
        let t5 = t.square() * t.square() * t;
        let xr_tmp = xr;
        if i < NROUNDS - 1 {
            xr = xl;
            xl = xr_tmp + t5;
        } else {
            xr = xr_tmp + t5;
        }
    }
    (xl, xr)
}

pub fn multi_hash(values: &[Fr], key: Fr, num_outputs: usize) -> Result<Vec<Fr>> {
    let mut r = Fr::zero();
    let mut c = Fr::zero();
    for value in values {
        r += *value;
        let state = sponge_hash(r, c, key);
        r = state.0;
        c = state.1;
    }
    let mut outputs = Vec::with_capacity(num_outputs);
    outputs.push(r);
    for _ in 1..num_outputs {
        let state = sponge_hash(r, c, key);
        r = state.0;
        c = state.1;
        outputs.push(r);
    }
    Ok(outputs)
}

pub fn hash_left_right(left: Fr, right: Fr) -> Result<Fr> {
    Ok(multi_hash(&[left, right], Fr::zero(), 1)?[0])
}

pub fn zero_subtree(level: usize) -> Result<Fr> {
    let mut value = Fr::zero();
    for _ in 0..level {
        value = hash_left_right(value, value)?;
    }
    Ok(value)
}

pub fn incremental_root(leaves: &[Fr]) -> Result<Fr> {
    let depth = super::merkle::MERKLE_TREE_DEPTH;
    if leaves.len() > (1usize << depth) {
        return Err(crate::Error::InvalidProof);
    }
    let mut filled = vec![Fr::zero(); depth];
    let mut root = zero_subtree(depth)?;
    for (index, leaf) in leaves.iter().enumerate() {
        let mut current_index = index;
        let mut current = *leaf;
        for level in 0..depth {
            if current_index % 2 == 0 {
                filled[level] = current;
                current = hash_left_right(current, zero_subtree(level)?)?;
            } else {
                current = hash_left_right(filled[level], current)?;
            }
            current_index /= 2;
        }
        root = current;
    }
    Ok(root)
}

/// Appends a single leaf to an incremental (Tornado-style) Merkle tree given the
/// persisted `filled` subtree nodes and the `next_index` (current leaf count),
/// returning the new root and the updated `filled` subtrees. This is O(depth) per
/// insert. Starting from `filled = [0; depth]` / `next_index = 0` and appending
/// leaves in order reproduces exactly the same roots as `incremental_root` over
/// the full leaf slice (same inner loop), so it stays consistent with the
/// circomlibjs-differential-tested full recompute.
pub fn append_leaf(filled: &[Fr], next_index: u64, leaf: Fr) -> Result<(Fr, Vec<Fr>)> {
    let depth = super::merkle::MERKLE_TREE_DEPTH;
    if filled.len() != depth {
        return Err(crate::Error::InvalidProof);
    }
    if next_index >= (1u64 << depth) {
        return Err(crate::Error::InvalidProof);
    }
    let mut filled = filled.to_vec();
    let mut current_index = next_index;
    let mut current = leaf;
    for (level, filled_level) in filled.iter_mut().enumerate() {
        if current_index % 2 == 0 {
            *filled_level = current;
            current = hash_left_right(current, zero_subtree(level)?)?;
        } else {
            current = hash_left_right(*filled_level, current)?;
        }
        current_index /= 2;
    }
    Ok((current, filled))
}

pub fn merkle_path(leaves: &[Fr], target_index: usize) -> Result<super::merkle::MerklePath> {
    if target_index >= leaves.len() {
        return Err(crate::Error::InvalidProof);
    }
    let depth = super::merkle::MERKLE_TREE_DEPTH;
    let mut path_elements = Vec::with_capacity(depth);
    let mut path_indices = Vec::with_capacity(depth);
    let mut level_nodes = leaves.to_vec();
    let mut current_index = target_index;
    for level in 0..depth {
        let sibling_index = current_index ^ 1;
        let sibling = level_nodes
            .get(sibling_index)
            .copied()
            .unwrap_or(zero_subtree(level)?);
        path_elements.push(fr_to_decimal(sibling));
        path_indices.push((current_index & 1) as u8);

        let mut next_level = Vec::with_capacity((level_nodes.len() + 1) / 2);
        for pair_index in (0..level_nodes.len()).step_by(2) {
            let left = level_nodes[pair_index];
            let right = level_nodes
                .get(pair_index + 1)
                .copied()
                .unwrap_or(zero_subtree(level)?);
            next_level.push(hash_left_right(left, right)?);
        }
        level_nodes = next_level;
        current_index /= 2;
    }
    Ok(super::merkle::MerklePath {
        leaf_index: target_index as u64,
        path_elements,
        path_indices,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::field::fr_from_decimal;

    #[test]
    fn round_constant_matches_ffjavascript() {
        let hex = "0fbe43c36a80e36d7c7c584d4f8f3759fb51f0d66065d8a227b688d12488c5d4";
        assert_eq!(
            super::super::field::fr_to_decimal(super::super::field::fr_from_hex(hex).unwrap()),
            "7120861356467848435263064379192047478074060781135320967663101236819528304084"
        );
    }

    #[test]
    fn incremental_root_rejects_over_capacity_leaf_count() {
        let depth = super::super::merkle::MERKLE_TREE_DEPTH;
        // One leaf beyond the depth-20 capacity must be rejected rather than silently
        // index-wrapping. The guard returns before any hashing, so this stays cheap.
        let leaves = vec![Fr::zero(); (1usize << depth) + 1];
        assert_eq!(incremental_root(&leaves), Err(crate::Error::InvalidProof));
    }

    #[test]
    fn append_leaf_matches_full_incremental_root() {
        let depth = super::super::merkle::MERKLE_TREE_DEPTH;
        let leaves: Vec<Fr> = (1u64..=5).map(Fr::from).collect();
        let mut filled = vec![Fr::zero(); depth];
        let mut root = Fr::zero();
        for (index, leaf) in leaves.iter().enumerate() {
            let (new_root, new_filled) = append_leaf(&filled, index as u64, *leaf).unwrap();
            filled = new_filled;
            root = new_root;
            // Appending 0..=index must equal a full recompute over the same prefix.
            assert_eq!(root, incremental_root(&leaves[..=index]).unwrap());
        }
        assert_eq!(root, incremental_root(&leaves).unwrap());
    }

    #[test]
    fn append_leaf_rejects_bad_state() {
        let depth = super::super::merkle::MERKLE_TREE_DEPTH;
        // Wrong filled length.
        assert_eq!(
            append_leaf(&[Fr::zero(); 3], 0, Fr::from(1u64)),
            Err(crate::Error::InvalidProof)
        );
        // next_index at capacity.
        let filled = vec![Fr::zero(); depth];
        assert_eq!(
            append_leaf(&filled, 1u64 << depth, Fr::from(1u64)),
            Err(crate::Error::InvalidProof)
        );
    }

    #[test]
    fn matches_circomlib_vectors() {
        let vectors: serde_json::Value =
            serde_json::from_str(include_str!("../../testdata/tornado_vectors.json")).unwrap();
        let case = &vectors["mimc"][0];
        let left = fr_from_decimal(case["left"].as_str().unwrap()).unwrap();
        let right = fr_from_decimal(case["right"].as_str().unwrap()).unwrap();
        assert_eq!(
            super::super::field::fr_to_decimal(hash_left_right(left, right).unwrap()),
            case["hash"].as_str().unwrap()
        );
    }
}
