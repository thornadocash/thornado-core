//! Groth16 proof verification against the production Tornado Cash v2.1 verification key.

use ark_bn254::{Bn254, Fq, Fq2, Fr, G1Affine, G2Affine};
use ark_ec::AffineRepr;
use ark_ff::PrimeField;
use ark_groth16::{Groth16, Proof, VerifyingKey};
use ark_snark::SNARK;
use num_bigint::BigUint;
use serde::{Deserialize, Serialize};

use super::ceremony::PRODUCTION_VK_JSON;
use crate::Result;

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct SnarkjsProof {
    pub pi_a: Vec<String>,
    pub pi_b: Vec<Vec<String>>,
    pub pi_c: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub protocol: Option<String>,
}

#[derive(Debug, Deserialize)]
struct SnarkjsVk {
    protocol: String,
    #[serde(rename = "nPublic")]
    n_public: usize,
    #[serde(rename = "vk_alfa_1")]
    vk_alpha_1: Vec<String>,
    #[serde(rename = "vk_beta_2")]
    vk_beta_2: Vec<Vec<String>>,
    #[serde(rename = "vk_gamma_2")]
    vk_gamma_2: Vec<Vec<String>>,
    #[serde(rename = "vk_delta_2")]
    vk_delta_2: Vec<Vec<String>>,
    #[serde(rename = "IC")]
    ic: Vec<Vec<String>>,
}

pub fn production_vk() -> Result<VerifyingKey<Bn254>> {
    static VK: std::sync::OnceLock<Result<VerifyingKey<Bn254>>> = std::sync::OnceLock::new();
    match VK.get_or_init(parse_production_vk) {
        Ok(vk) => Ok(vk.clone()),
        Err(_) => Err(crate::Error::InvalidProof),
    }
}

fn parse_production_vk() -> Result<VerifyingKey<Bn254>> {
    let parsed: SnarkjsVk =
        serde_json::from_str(PRODUCTION_VK_JSON).map_err(|_| crate::Error::InvalidProof)?;
    if parsed.protocol != "groth" || parsed.n_public != 6 {
        return Err(crate::Error::InvalidProof);
    }
    Ok(VerifyingKey {
        alpha_g1: parse_g1(&parsed.vk_alpha_1)?,
        beta_g2: parse_g2(&parsed.vk_beta_2)?,
        gamma_g2: parse_g2(&parsed.vk_gamma_2)?,
        delta_g2: parse_g2(&parsed.vk_delta_2)?,
        gamma_abc_g1: parsed
            .ic
            .iter()
            .map(|point| parse_g1(point))
            .collect::<Result<_>>()?,
    })
}

pub fn verify_snarkjs_proof(proof: &SnarkjsProof, public_inputs: &[Fr]) -> Result<()> {
    if proof.pi_a.len() < 2 || proof.pi_c.len() < 2 || proof.pi_b.len() < 2 {
        return Err(crate::Error::InvalidProof);
    }
    let vk = production_vk()?;
    if public_inputs.len() + 1 != vk.gamma_abc_g1.len() {
        return Err(crate::Error::InvalidProof);
    }
    let proof = Proof::<Bn254> {
        a: parse_g1(&proof.pi_a)?,
        b: parse_g2(&proof.pi_b)?,
        c: parse_g1(&proof.pi_c)?,
    };
    Groth16::<Bn254>::verify(&vk, public_inputs, &proof)
        .map_err(|_| crate::Error::InvalidProof)
        .and_then(|ok| {
            if ok {
                Ok(())
            } else {
                Err(crate::Error::InvalidProof)
            }
        })
}

fn parse_g1(values: &[String]) -> Result<G1Affine> {
    if values.len() < 2 {
        return Err(crate::Error::InvalidProof);
    }
    let x = parse_fq(&values[0])?;
    let y = parse_fq(&values[1])?;
    let point = G1Affine::new(x, y);
    if point.is_zero() {
        return Ok(G1Affine::identity());
    }
    Ok(point)
}

fn parse_g2(values: &[Vec<String>]) -> Result<G2Affine> {
    if values.len() < 2 {
        return Err(crate::Error::InvalidProof);
    }
    let x = Fq2::new(parse_fq(&values[0][0])?, parse_fq(&values[0][1])?);
    let y = Fq2::new(parse_fq(&values[1][0])?, parse_fq(&values[1][1])?);
    let point = G2Affine::new(x, y);
    if point.is_zero() {
        return Ok(G2Affine::identity());
    }
    Ok(point)
}

fn parse_fq(decimal: &str) -> Result<Fq> {
    let trimmed = decimal.trim();
    let mut value = BigUint::from(0u8);
    for ch in trimmed.as_bytes() {
        if !ch.is_ascii_digit() {
            return Err(crate::Error::InvalidProof);
        }
        value *= 10u32;
        value += (*ch - b'0') as u32;
    }
    let modulus: BigUint = Fq::MODULUS.into();
    let reduced: BigUint = value % modulus;
    let mut repr = <Fq as PrimeField>::BigInt::zero();
    let bytes = reduced.to_bytes_le();
    for (idx, byte) in bytes.into_iter().enumerate().take(48) {
        let limb = idx / 8;
        let shift = (idx % 8) * 8;
        repr.as_mut()[limb] |= (byte as u64) << shift;
    }
    Fq::from_bigint(repr).ok_or(crate::Error::InvalidProof)
}

pub fn public_inputs_from_withdraw(
    root: Fr,
    nullifier_hash: Fr,
    recipient: Fr,
    relayer: Fr,
    fee: Fr,
    refund: Fr,
) -> [Fr; 6] {
    [root, nullifier_hash, recipient, relayer, fee, refund]
}

pub fn vk_digest_hex() -> String {
    use sha2::{Digest, Sha256};
    hex::encode(Sha256::digest(PRODUCTION_VK_JSON.as_bytes()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tornado::ceremony;

    #[test]
    fn production_vk_loads() {
        production_vk().unwrap();
        assert_eq!(vk_digest_hex(), ceremony::verification_key_sha256());
    }
}
