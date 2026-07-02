//! MiMC Merkle tree (depth 20) matching tornado-core incremental insertion.

use ark_bn254::Fr;
use ark_ff::Zero;

use super::field::{fr_from_decimal, fr_from_field_hex, fr_to_hex};
use super::mimc_sponge;
use crate::Result;
use serde::{Deserialize, Serialize};

pub const MERKLE_TREE_DEPTH: usize = 20;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct MerklePath {
    pub leaf_index: u64,
    pub path_elements: Vec<String>,
    pub path_indices: Vec<u8>,
}

pub fn zero_leaf() -> Fr {
    Fr::zero()
}

pub fn zero_subtree(level: usize) -> Result<Fr> {
    mimc_sponge::zero_subtree(level)
}

pub fn merkle_root_hex(leaves: &[String]) -> Result<String> {
    let parsed: Result<Vec<Fr>> = leaves
        .iter()
        .map(|leaf| fr_from_field_hex(leaf).ok_or(crate::Error::InvalidProof))
        .collect();
    Ok(fr_to_hex(incremental_root(&parsed?)?))
}

pub fn incremental_root(leaves: &[Fr]) -> Result<Fr> {
    mimc_sponge::incremental_root(leaves)
}

pub fn append_leaf(filled: &[Fr], next_index: u64, leaf: Fr) -> Result<(Fr, Vec<Fr>)> {
    mimc_sponge::append_leaf(filled, next_index, leaf)
}

pub fn merkle_path(leaves: &[Fr], leaf_index: usize) -> Result<MerklePath> {
    mimc_sponge::merkle_path(leaves, leaf_index)
}

pub fn verify_merkle_path(root_hex: &str, leaf_hex: &str, path: &MerklePath) -> Result<()> {
    let root = fr_from_field_hex(root_hex).ok_or(crate::Error::InvalidProof)?;
    let mut current = fr_from_field_hex(leaf_hex).ok_or(crate::Error::InvalidProof)?;
    if path.path_elements.len() != MERKLE_TREE_DEPTH || path.path_indices.len() != MERKLE_TREE_DEPTH
    {
        return Err(crate::Error::InvalidProof);
    }
    for (sibling_raw, direction) in path.path_elements.iter().zip(path.path_indices.iter()) {
        let sibling = fr_from_decimal(sibling_raw).ok_or(crate::Error::InvalidProof)?;
        current = if *direction == 0 {
            mimc_sponge::hash_left_right(current, sibling)?
        } else {
            mimc_sponge::hash_left_right(sibling, current)?
        };
    }
    if current != root {
        return Err(crate::Error::InvalidProof);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::field::fr_from_decimal;
    use crate::tornado::field::fr_to_decimal;

    #[test]
    fn matches_circomlib_vectors() {
        let vectors: serde_json::Value =
            serde_json::from_str(include_str!("../../testdata/tornado_vectors.json")).unwrap();
        let case = &vectors["merkle"][0];
        let leaves: Vec<Fr> = case["leaves"]
            .as_array()
            .unwrap()
            .iter()
            .map(|leaf| fr_from_decimal(leaf.as_str().unwrap()).unwrap())
            .collect();
        assert_eq!(
            fr_to_decimal(incremental_root(&leaves).unwrap()),
            case["root"].as_str().unwrap()
        );
    }

    #[test]
    fn merkle_path_verifies_vector_root() {
        let vectors: serde_json::Value =
            serde_json::from_str(include_str!("../../testdata/tornado_vectors.json")).unwrap();
        let case = &vectors["merkle"][0];
        let leaves: Vec<Fr> = case["leaves"]
            .as_array()
            .unwrap()
            .iter()
            .map(|leaf| fr_from_decimal(leaf.as_str().unwrap()).unwrap())
            .collect();
        let path = merkle_path(&leaves, 0).unwrap();
        verify_merkle_path(
            &fr_to_hex(fr_from_decimal(case["root"].as_str().unwrap()).unwrap()),
            &fr_to_hex(leaves[0]),
            &path,
        )
        .unwrap();
    }
}
