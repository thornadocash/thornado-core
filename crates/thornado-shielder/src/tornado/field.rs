//! BN254 scalar field helpers (Tornado Cash public inputs / note fields).

use ark_bn254::Fr;
use ark_ff::{BigInteger, PrimeField, Zero};
use num_bigint::BigUint;

pub const PUBLIC_INPUT_COUNT: usize = 6;

fn bytes_to_fr_repr(bytes: impl IntoIterator<Item = u8>) -> <Fr as PrimeField>::BigInt {
    let mut repr = <Fr as PrimeField>::BigInt::zero();
    for (idx, byte) in bytes.into_iter().enumerate().take(32) {
        let limb = idx / 8;
        let shift = (idx % 8) * 8;
        repr.as_mut()[limb] |= (byte as u64) << shift;
    }
    repr
}

pub fn fr_from_le_bytes(bytes: &[u8]) -> Option<Fr> {
    let mut value = BigUint::from(0u8);
    for (idx, byte) in bytes.iter().enumerate().take(32) {
        value += BigUint::from(*byte) << (8 * idx);
    }
    let modulus: BigUint = Fr::MODULUS.into();
    let reduced = value % modulus;
    Fr::from_bigint(bytes_to_fr_repr(reduced.to_bytes_le()))
}

pub fn fr_from_be_bytes(bytes: &[u8]) -> Option<Fr> {
    let mut value = BigUint::from(0u8);
    for byte in bytes {
        value = (value << 8) | BigUint::from(*byte);
    }
    let modulus: BigUint = Fr::MODULUS.into();
    let reduced = value % modulus;
    Fr::from_bigint(bytes_to_fr_repr(reduced.to_bytes_le()))
}

pub fn fr_from_decimal(decimal: &str) -> Option<Fr> {
    let trimmed = decimal.trim();
    if trimmed.is_empty() {
        return None;
    }
    let mut value = BigUint::from(0u8);
    for ch in trimmed.as_bytes() {
        if !ch.is_ascii_digit() {
            return None;
        }
        value *= 10u32;
        value += (*ch - b'0') as u32;
    }
    let modulus: BigUint = Fr::MODULUS.into();
    let reduced = value % modulus;
    Fr::from_bigint(bytes_to_fr_repr(reduced.to_bytes_le()))
}

pub fn fr_from_hex(hex_str: &str) -> Option<Fr> {
    let trimmed = hex_str.trim().trim_start_matches("0x");
    if trimmed.is_empty() {
        return None;
    }
    let mut value = BigUint::from(0u8);
    for pair in trimmed.as_bytes().chunks(2) {
        if pair.len() != 2 {
            return None;
        }
        let hi = from_hex_digit(pair[0])?;
        let lo = from_hex_digit(pair[1])?;
        value = (value << 8) | BigUint::from(hi * 16 + lo);
    }
    let modulus: BigUint = Fr::MODULUS.into();
    let reduced = value % modulus;
    Fr::from_bigint(bytes_to_fr_repr(reduced.to_bytes_le()))
}

fn from_hex_digit(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

pub fn fr_to_decimal(value: Fr) -> String {
    value.into_bigint().to_string()
}

pub fn fr_to_hex(value: Fr) -> String {
    let repr = value.into_bigint().to_bytes_le();
    let mut bytes = [0_u8; 32];
    bytes[..repr.len().min(32)].copy_from_slice(&repr[..repr.len().min(32)]);
    hex::encode(bytes)
}

pub fn fr_from_field_hex(hex_str: &str) -> Option<Fr> {
    let trimmed = hex_str.trim().trim_start_matches("0x");
    if trimmed.is_empty() {
        return Some(Fr::zero());
    }
    let bytes = hex::decode(trimmed).ok()?;
    fr_from_le_bytes(&bytes)
}

pub fn bits248_le(value: Fr) -> [bool; 248] {
    let repr = value.into_bigint().to_bytes_le();
    let mut bits = [false; 248];
    for (idx, bit) in bits.iter_mut().enumerate() {
        let byte = repr.get(idx / 8).copied().unwrap_or(0);
        *bit = ((byte >> (idx % 8)) & 1) == 1;
    }
    bits
}

pub fn u64_to_fr(value: u64) -> Fr {
    Fr::from(value)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn roundtrip_decimal() {
        let value = fr_from_hex("0101010101010101010101010101010101010101010101010101010101010101")
            .unwrap();
        let decimal = fr_to_decimal(value);
        assert_eq!(fr_from_decimal(&decimal).unwrap(), value);
    }

    #[test]
    fn hex_matches_ffjavascript_decimal() {
        let value = fr_from_hex("0101010101010101010101010101010101010101010101010101010101010101")
            .unwrap();
        assert_eq!(
            fr_to_decimal(value),
            "454086624460063511464984254936031011189294057512315937409637584344757371137"
        );
    }
}
