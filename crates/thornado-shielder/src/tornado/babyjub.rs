//! Baby Jubjub curve (circomlib / bn254), matching circomlibjs `babyjub.js`.

use ark_bn254::Fr;
use ark_ff::{BigInteger, Field, LegendreSymbol, One, PrimeField, Zero};
use num_bigint::BigUint;
use num_traits::{Num, ToPrimitive};

const A: u64 = 168_700;
const D: u64 = 168_696;

type Point = (Fr, Fr);

fn identity() -> Point {
    (Fr::zero(), Fr::one())
}

fn add_point(a: Point, b: Point) -> Point {
    let a_coef = Fr::from(A);
    let d_coef = Fr::from(D);
    let beta = a.0 * b.1;
    let gamma = a.1 * b.0;
    let delta = (a.1 - a_coef * a.0) * (b.0 + b.1);
    let tau = beta * gamma;
    let dtau = d_coef * tau;
    let x = (beta + gamma) / (Fr::one() + dtau);
    let y = (delta + a_coef * beta - gamma) / (Fr::one() - dtau);
    (x, y)
}

fn mul_point_escalar(mut base: Point, mut scalar: BigUint) -> Point {
    let mut res = identity();
    while !scalar.is_zero() {
        if (&scalar & BigUint::one()).to_u32().unwrap() == 1 {
            res = add_point(res, base);
        }
        base = add_point(base, base);
        scalar >>= 1u32;
    }
    res
}

fn in_curve(p: Point) -> bool {
    let a_coef = Fr::from(A);
    let d_coef = Fr::from(D);
    let x2 = p.0.square();
    let y2 = p.1.square();
    a_coef * x2 + y2 == Fr::one() + d_coef * x2 * y2
}

fn suborder() -> BigUint {
    BigUint::from_str_radix(
        "21888242871839275222246405745257275088614511777268538073601725287587578984328",
        10,
    )
    .expect("suborder decimal")
        >> 3u32
}

fn in_subgroup(p: Point) -> bool {
    if !in_curve(p) {
        return false;
    }
    let scaled = mul_point_escalar(p, suborder());
    scaled.0.is_zero() && scaled.1 == Fr::one()
}

fn unpack_point(mut bytes: [u8; 32]) -> Option<Point> {
    let mut sign = false;
    if bytes[31] & 0x80 != 0 {
        sign = true;
        bytes[31] &= 0x7f;
    }
    let y = {
        let mut repr = <Fr as PrimeField>::BigInt::zero();
        for (idx, byte) in bytes.iter().enumerate().take(32) {
            let limb = idx / 8;
            let shift = (idx % 8) * 8;
            repr.as_mut()[limb] |= (*byte as u64) << shift;
        }
        Fr::from_bigint(repr)?
    };
    let a_coef = Fr::from(A);
    let d_coef = Fr::from(D);
    let y2 = y.square();
    let x2 = (Fr::one() - y2) / (a_coef - d_coef * y2);
    if !matches!(
        x2.legendre(),
        LegendreSymbol::QuadraticResidue | LegendreSymbol::Zero
    ) {
        return None;
    }
    let mut x = x2.sqrt().expect("x2 is quadratic residue");
    let x_int: BigUint = x.into_bigint().into();
    let modulus: BigUint = Fr::MODULUS.into();
    let half = (&modulus - BigUint::one()) >> 1u32;
    if x_int > half {
        x = -x;
    }
    if sign {
        x = -x;
    }
    Some((x, y))
}

pub fn point_x_from_packed(packed: [u8; 32]) -> Option<Fr> {
    let (x, _) = unpack_point(packed)?;
    Some(x)
}

pub fn mul_base_point_escalar(base: Point, scalar: BigUint) -> Point {
    mul_point_escalar(base, scalar)
}

pub fn add_points(a: Point, b: Point) -> Point {
    add_point(a, b)
}

pub fn zero_point() -> Point {
    identity()
}

fn generator_base_point(base_hash: &[u8; 32]) -> Option<Point> {
    let mut bytes = *base_hash;
    bytes[31] &= 0xbf;
    let point = unpack_point(bytes)?;
    let scaled = mul_point_escalar(point, BigUint::from(8u8));
    in_subgroup(scaled).then_some(scaled)
}

pub fn pack_point(p: Point) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    let y_repr = p.1.into_bigint().to_bytes_le();
    bytes[..y_repr.len().min(32)].copy_from_slice(&y_repr[..y_repr.len().min(32)]);
    let x_int: BigUint = p.0.into_bigint().into();
    let modulus: BigUint = Fr::MODULUS.into();
    let half = (&modulus - BigUint::one()) >> 1u32;
    if x_int > half {
        bytes[31] |= 0x80;
    }
    bytes
}

pub fn derive_base_point(point_idx: u32, try_idx: u32) -> [u8; 32] {
    use blake_hash::Blake256;
    use digest08::Digest;
    let seed = format!("PedersenGenerator_{:032}_{:032}", point_idx, try_idx);
    let digest = Blake256::digest(seed.as_bytes());
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

pub fn get_base_point(point_idx: u32) -> Option<Point> {
    for try_idx in 0..256 {
        let hash = derive_base_point(point_idx, try_idx);
        if let Some(point) = generator_base_point(&hash) {
            return Some(point);
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identity_is_on_curve() {
        assert!(in_curve(identity()));
    }
}
