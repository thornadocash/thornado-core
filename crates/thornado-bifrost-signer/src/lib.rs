//! Pure-Rust FROST signer bifrost.
//!
//! Replaces the Go bifrost signing path (signer orchestration, FROST p2p
//! sessions, BTC transaction construction) with a single-language stack on
//! top of `frost-secp256k1-tr` — no FFI boundary.

pub mod attestation;
pub mod bitcoind;
pub mod broadcast;
pub mod chain;
pub mod daemon;
pub mod extract;
pub mod frost_session;
pub mod observer;
pub mod p2p;
pub mod scanner;
pub mod signer;
pub mod store;
pub mod temporal;
pub mod transport;
pub mod tx_builder;
pub mod wire;
