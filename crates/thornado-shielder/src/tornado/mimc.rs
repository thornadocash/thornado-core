//! MiMC sponge pair hash (production circomlib semantics).

use ark_bn254::Fr;

use super::mimc_sponge;
use crate::Result;

pub fn hash_left_right(left: Fr, right: Fr) -> Result<Fr> {
    mimc_sponge::hash_left_right(left, right)
}

pub fn incremental_root(leaves: &[Fr]) -> Result<Fr> {
    mimc_sponge::incremental_root(leaves)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::field::{fr_from_decimal, fr_to_decimal};

    #[test]
    fn matches_circomlib_vectors() {
        let vectors: serde_json::Value =
            serde_json::from_str(include_str!("../../testdata/tornado_vectors.json")).unwrap();
        let case = &vectors["mimc"][0];
        let left = fr_from_decimal(case["left"].as_str().unwrap()).unwrap();
        let right = fr_from_decimal(case["right"].as_str().unwrap()).unwrap();
        assert_eq!(
            fr_to_decimal(hash_left_right(left, right).unwrap()),
            case["hash"].as_str().unwrap()
        );
    }
}
