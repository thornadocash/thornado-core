//! Tornado Cash note hashing (Pedersen over 248-bit limbs).

use ark_bn254::Fr;

use super::field::fr_to_decimal;
use super::node_crypto;
use crate::Result;

pub use super::field::{fr_from_hex as field_from_hex, fr_to_hex as field_to_hex};

pub type Fp = Fr;

pub fn note_commitment(nullifier: Fr, secret: Fr) -> Result<Fr> {
    node_crypto::note_commitment(nullifier, secret)
}

pub fn nullifier_hash(nullifier: Fr) -> Result<Fr> {
    node_crypto::nullifier_hash(nullifier)
}

pub fn recipient_binding_decimal(
    recipient: &str,
    fee_sats: u64,
    denomination_sats: u64,
) -> Result<String> {
    Ok(fr_to_decimal(recipient_binding(
        recipient,
        fee_sats,
        denomination_sats,
    )?))
}

pub fn recipient_binding(recipient: &str, fee_sats: u64, denomination_sats: u64) -> Result<Fr> {
    let digest = crate::hash_parts_bytes(&[
        crate::DOMAIN,
        "tornado-recipient-binding",
        recipient,
        &fee_sats.to_string(),
        &denomination_sats.to_string(),
    ]);
    super::field::fr_from_be_bytes(&digest).ok_or(crate::Error::InvalidProof)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::field::fr_from_hex;

    #[test]
    fn recipient_binding_reduces_mod_field() {
        let value = recipient_binding("bcrt1qrecipient", 1_000, 100_000).unwrap();
        assert!(!fr_to_decimal(value).is_empty());
    }

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
