//! Block-transaction extraction: turning a decoded Bitcoin transaction into an
//! inbound observation (`TxInItem`).
//!
//! Ports the pure core of `client_internal.go` — `extractTxs` / `getTxIns` /
//! `getOutput(s)` / `getGas` and the scriptPubKey → address decoding
//! (`getAddressesFromScriptPubKey`). rust-bitcoin is the oracle for all
//! Bitcoin encoding/decoding; the Go code is the behavioural reference.
//!
//! The chain-facing plumbing (RPC fan-out, mempool cache, temporal dedupe) is
//! left to the caller. What lives here is deterministic and unit-testable: the
//! caller supplies a decoded tx plus vault-membership closures, and gets back an
//! observation (or `None` when nothing matches).

use bitcoin::{Address, Network, ScriptBuf};

use crate::observer::{self, ObserveError, Vout};

/// Native gas asset ticker recorded on the coin/gas of a BTC observation.
pub const BTC_GAS_ASSET: &str = "BTC.BTC";

#[derive(Debug, thiserror::Error)]
pub enum ExtractError {
    #[error("invalid scriptPubKey hex: {0}")]
    BadHex(String),
    #[error("output value out of range")]
    BadAmount,
    #[error("observation selection: {0}")]
    Observe(#[from] ObserveError),
}

/// A single decoded transaction input, carrying the prevout address it spends
/// from and the prevout amount (needed for the sender and for gas).
#[derive(Debug, Clone)]
pub struct DecodedInput {
    /// The prevout's referenced txid (Go `vin[0].txid`; empty => structural
    /// ignore).
    pub prev_txid: String,
    /// Prevout output index.
    pub prev_vout: u32,
    /// Address decoded from the prevout scriptPubKey, if single-sig/standard.
    pub prev_address: Option<String>,
    /// Prevout value in satoshis.
    pub prev_amount_sats: u64,
}

/// A single decoded transaction output: raw value (satoshis) and its
/// scriptPubKey as hex.
#[derive(Debug, Clone)]
pub struct DecodedOutput {
    /// Output index within the tx (Go `vout.N`).
    pub n: u32,
    pub value_sats: u64,
    pub script_hex: String,
}

/// A spent UTXO reference carried on observations (Go `common.TxInput`).
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct SourceInput {
    pub tx_id: String,
    pub vout: u32,
    pub amount_sats: u64,
}

/// An amount of a single asset, in satoshis.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct Coin {
    pub asset: String,
    pub amount_sats: u64,
}

/// The inbound-relevant subset of the Go `TxInItem` observation.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct TxInItem {
    pub block_height: i64,
    /// The observed tx's txid.
    pub tx: String,
    /// Sender address (from vin:0's prevout).
    pub sender: String,
    /// Recipient address of the observed output.
    pub to: String,
    /// The observed coins (asset + amount), one entry for BTC.
    pub coins: Vec<Coin>,
    /// Network fee: sum(inputs) - sum(outputs), as gas coins.
    pub gas: Vec<Coin>,
    /// The vault pubkey this observation is attributed to (bech32/hex string;
    /// caller resolves it from `to`). Empty when unknown.
    #[serde(default)]
    pub observed_vault_pubkey: String,
    /// The UTXOs the observed tx spent (Go `SourceInputs`) — the chain uses
    /// these for vault UTXO bookkeeping and internal-outbound matching.
    #[serde(default)]
    pub source_inputs: Vec<SourceInput>,
    /// Index of the observed output within the source tx (Go `SourceVout`).
    pub source_vout: u32,
}

/// Convert a floating-point BTC amount to satoshis, matching Go
/// `btcutil.NewAmount(v).ToUnit(AmountSatoshi)` (round-half-away-from-zero).
pub fn btc_to_sats(btc: f64) -> u64 {
    (btc * 1e8).round() as u64
}

/// Decode a scriptPubKey (hex) into its address string(s), using rust-bitcoin's
/// `Address::from_script`. Supports P2TR, P2WPKH, P2WSH, P2PKH and P2SH. Returns
/// an empty vec for nonstandard / nulldata (OP_RETURN) scripts.
///
/// Replaces Go `getAddressesFromScriptPubKey` (which fell through to
/// `txscript.ExtractPkScriptAddrs`): rust-bitcoin yields at most one address for
/// the standard single-recipient scripts we observe, so a returned vec of
/// length != 1 signals "not a single-recipient output" to the selection rule.
pub fn decode_addresses(script_pubkey_hex: &str, network: Network) -> Vec<String> {
    let bytes = match hex::decode(script_pubkey_hex) {
        Ok(b) => b,
        Err(_) => return Vec::new(),
    };
    let script = ScriptBuf::from_bytes(bytes);
    match Address::from_script(script.as_script(), network) {
        Ok(addr) => vec![addr.to_string()],
        Err(_) => Vec::new(),
    }
}

/// Build the observer-layer `Vout` view of a decoded output: value in BTC, the
/// decoded receiver address(es), and a script "type" tag good enough for the
/// structural pre-filter (only "nulldata" is treated specially there).
fn to_observer_vout(out: &DecodedOutput, network: Network) -> Vout {
    let addresses = decode_addresses(&out.script_hex, network);
    // An empty address set with a nonzero-length script is either an OP_RETURN
    // (nulldata) or otherwise nonstandard; tag nulldata so should_ignore_tx can
    // discount it. Everything with an address is "standard".
    let script_type = if addresses.is_empty() {
        "nulldata".to_string()
    } else {
        "standard".to_string()
    };
    Vout {
        value: out.value_sats as f64 / 1e8,
        addresses,
        script_type,
    }
}

/// Extract the inbound observation from a decoded transaction.
///
/// Mirrors the pure core of Go `getTxIns` + `txInItemFromOutput`:
///
/// 1. structural pre-filter (`observer::should_ignore_tx`);
/// 2. pick the observable output via `observer::select_output`
///    (`consolidate = is_protocol_controlled(sender)`, matching Go's use of the
///    consolidate branch for protocol senders);
/// 3. build the `TxInItem` with coins = the output amount and
///    gas = sum(inputs) - sum(outputs).
///
/// `is_vault` marks base (vault) addresses; `is_protocol_controlled` marks
/// protocol-controlled sender addresses. Outputs below `dust_threshold_sats` are
/// skipped. Returns `Ok(None)` when nothing matches the observation criteria.
#[allow(clippy::too_many_arguments)]
pub fn extract_observation<V, P>(
    block_height: i64,
    txid: &str,
    inputs: &[DecodedInput],
    outputs: &[DecodedOutput],
    sender: &str,
    is_vault: V,
    is_protocol_controlled: P,
    dust_threshold_sats: u64,
    network: Network,
) -> Result<Option<TxInItem>, ExtractError>
where
    V: Fn(&str) -> bool,
    P: Fn(&str) -> bool,
{
    extract_observation_capped(
        block_height,
        txid,
        inputs,
        outputs,
        sender,
        is_vault,
        is_protocol_controlled,
        dust_threshold_sats,
        network,
        observer::MAX_VALUE_OUTPUTS,
        |a, b| a == b,
    )
}

/// [`extract_observation`] with an explicit output-count cap (see
/// [`observer::should_ignore_tx_capped`]).
#[allow(clippy::too_many_arguments)]
pub fn extract_observation_capped<V, P, S>(
    block_height: i64,
    txid: &str,
    inputs: &[DecodedInput],
    outputs: &[DecodedOutput],
    sender: &str,
    is_vault: V,
    is_protocol_controlled: P,
    dust_threshold_sats: u64,
    network: Network,
    max_value_outputs: usize,
    same_vault: S,
) -> Result<Option<TxInItem>, ExtractError>
where
    V: Fn(&str) -> bool,
    P: Fn(&str) -> bool,
    S: Fn(&str, &str) -> bool,
{
    let vins: Vec<observer::Vin> = inputs
        .iter()
        .map(|i| observer::Vin {
            txid: i.prev_txid.clone(),
        })
        .collect();
    let vouts: Vec<Vout> = outputs
        .iter()
        .map(|o| to_observer_vout(o, network))
        .collect();

    if observer::should_ignore_tx_capped(&vins, &vouts, max_value_outputs) {
        return Ok(None);
    }

    let source_inputs: Vec<SourceInput> = inputs
        .iter()
        .map(|i| SourceInput {
            tx_id: i.prev_txid.clone(),
            vout: i.prev_vout,
            amount_sats: i.prev_amount_sats,
        })
        .collect();
    let sum_in: u64 = inputs.iter().map(|i| i.prev_amount_sats).sum();
    let sum_out: u64 = outputs.iter().map(|o| o.value_sats).sum();
    let gas_sats = sum_in.saturating_sub(sum_out);

    // Vault-sent txs are observed as ONE batch outbound covering every output
    // that is not change back to the sender: to = the first recipient, amount
    // = the recipient total (Go `getBatchOutboundTxIn`). Only when there is no
    // such output (pure consolidation) does the consolidate branch apply.
    if is_protocol_controlled(sender) {
        let mut to_addr = String::new();
        let mut total = 0u64;
        for (i, v) in vouts.iter().enumerate() {
            if outputs[i].value_sats == 0 || v.addresses.len() != 1 {
                continue;
            }
            let receiver = &v.addresses[0];
            if receiver.eq_ignore_ascii_case(sender) || same_vault(sender, receiver) {
                continue;
            }
            if to_addr.is_empty() {
                to_addr = receiver.clone();
            }
            total += outputs[i].value_sats;
        }
        if !to_addr.is_empty() {
            return Ok(Some(TxInItem {
                block_height,
                tx: txid.to_string(),
                sender: sender.to_string(),
                to: to_addr,
                coins: vec![Coin {
                    asset: BTC_GAS_ASSET.to_string(),
                    amount_sats: total,
                }],
                gas: vec![Coin {
                    asset: BTC_GAS_ASSET.to_string(),
                    amount_sats: gas_sats,
                }],
                observed_vault_pubkey: String::new(),
                source_inputs,
                source_vout: 0,
            }));
        }
    }

    // Protocol senders take the consolidate branch (self-outputs); customer
    // sends take the normal branch (outputs to a vault). Matches Go getTxIns,
    // which calls getOutput(sender, tx, consolidate=true) for protocol senders.
    let consolidate = is_protocol_controlled(sender);
    let selected = match observer::select_outputs_vault_aware(
        sender,
        &vouts,
        consolidate,
        &is_protocol_controlled,
        &is_vault,
        &same_vault,
    )
    .map(|v| v[0])
    {
        Ok(idx) => idx,
        Err(ObserveError::NoOutputMatch) => return Ok(None),
    };

    let out = &outputs[selected];
    if out.value_sats < dust_threshold_sats {
        return Ok(None);
    }

    let to = vouts[selected]
        .addresses
        .first()
        .cloned()
        .ok_or(ExtractError::BadAmount)?;

    let item = TxInItem {
        block_height,
        tx: txid.to_string(),
        sender: sender.to_string(),
        to,
        coins: vec![Coin {
            asset: BTC_GAS_ASSET.to_string(),
            amount_sats: out.value_sats,
        }],
        gas: vec![Coin {
            asset: BTC_GAS_ASSET.to_string(),
            amount_sats: gas_sats,
        }],
        observed_vault_pubkey: String::new(),
        source_inputs,
        source_vout: out.n,
    };
    Ok(Some(item))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tx_builder::TaprootVault;

    const NET: Network = Network::Bitcoin;

    /// A P2TR scriptPubKey hex for a vault with the given 32-byte output key.
    fn p2tr_hex(key_byte: u8) -> String {
        let vault = TaprootVault::from_output_key([key_byte; 32]);
        hex::encode(vault.script_pubkey().as_bytes())
    }

    /// The address rust-bitcoin decodes that P2TR script to.
    fn p2tr_addr(key_byte: u8) -> String {
        decode_addresses(&p2tr_hex(key_byte), NET)
            .into_iter()
            .next()
            .expect("p2tr decodes")
    }

    fn out(n: u32, sats: u64, script_hex: &str) -> DecodedOutput {
        DecodedOutput {
            n,
            value_sats: sats,
            script_hex: script_hex.to_string(),
        }
    }

    fn input(txid: &str, addr: &str, sats: u64) -> DecodedInput {
        DecodedInput {
            prev_txid: txid.to_string(),
            prev_vout: 0,
            prev_address: if addr.is_empty() {
                None
            } else {
                Some(addr.to_string())
            },
            prev_amount_sats: sats,
        }
    }

    #[test]
    fn btc_to_sats_rounds_half_away() {
        assert_eq!(btc_to_sats(1.0), 100_000_000);
        assert_eq!(btc_to_sats(0.000_000_01), 1);
        // 1.234_567_895 BTC -> 123456789.5 sats -> rounds to 123456790
        assert_eq!(btc_to_sats(1.234_567_895), 123_456_790);
        assert_eq!(btc_to_sats(0.0), 0);
    }

    #[test]
    fn decode_p2tr_is_bech32m() {
        let addr = p2tr_addr(9);
        assert!(addr.starts_with("bc1p"), "got {addr}");
        // single address returned
        assert_eq!(decode_addresses(&p2tr_hex(9), NET).len(), 1);
    }

    #[test]
    fn decode_p2pkh() {
        // OP_DUP OP_HASH160 <20> ... OP_EQUALVERIFY OP_CHECKSIG (mainnet 1...)
        let hex = "76a91415fb126815935f6ae83a206d7d82f1065bc63e2588ac";
        let addrs = decode_addresses(hex, NET);
        assert_eq!(addrs.len(), 1);
        assert!(addrs[0].starts_with('1'), "got {}", addrs[0]);
    }

    #[test]
    fn decode_p2sh() {
        let hex = "a914e51a3dd98ded55718ad2cf2ce7c8ff056394445787";
        let addrs = decode_addresses(hex, NET);
        assert_eq!(addrs.len(), 1);
        assert!(addrs[0].starts_with('3'), "got {}", addrs[0]);
    }

    #[test]
    fn decode_p2wpkh() {
        let hex = "00140653096f54ae1ae2d73291d15854aef08ebcfa8c";
        let addrs = decode_addresses(hex, NET);
        assert_eq!(addrs.len(), 1);
        assert!(addrs[0].starts_with("bc1q"), "got {}", addrs[0]);
    }

    #[test]
    fn decode_nulldata_is_empty() {
        // OP_RETURN <push> -> nonstandard for Address::from_script
        let hex = "6a0b68656c6c6f20776f726c64"; // OP_RETURN "hello world"
        assert!(decode_addresses(hex, NET).is_empty());
    }

    #[test]
    fn decode_bad_hex_is_empty() {
        assert!(decode_addresses("zzzz", NET).is_empty());
    }

    #[test]
    fn inbound_to_vault_builds_observation() {
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let change_hex = p2tr_hex(2);
        let change_addr = p2tr_addr(2);

        // customer (change_addr sender) sends 0.5 BTC to vault, 0.19 change.
        let inputs = vec![input("aa", &change_addr, 70_000_000)];
        let outputs = vec![
            out(0, 50_000_000, &vault_hex),
            out(1, 19_000_000, &change_hex),
        ];

        let item = extract_observation(
            100,
            "deadbeef",
            &inputs,
            &outputs,
            &change_addr, // sender == the change/customer addr
            |a| a == vault,
            |_| false, // sender not protocol-controlled
            10_000,
            NET,
        )
        .unwrap()
        .expect("observation");

        assert_eq!(item.block_height, 100);
        assert_eq!(item.tx, "deadbeef");
        assert_eq!(item.to, vault);
        assert_eq!(item.source_vout, 0);
        assert_eq!(item.coins[0].amount_sats, 50_000_000);
        assert_eq!(item.coins[0].asset, BTC_GAS_ASSET);
        // gas = 70_000_000 - (50_000_000 + 19_000_000) = 1_000_000
        assert_eq!(item.gas[0].amount_sats, 1_000_000);
    }

    #[test]
    fn outbound_from_vault_picks_recipient_not_change() {
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let recipient = p2tr_addr(3);
        let recipient_hex = p2tr_hex(3);

        let inputs = vec![input("bb", &vault, 100_000_000)];
        // recipient output then change back to vault (self)
        let outputs = vec![
            out(0, 30_000_000, &recipient_hex),
            out(1, 69_000_000, &vault_hex),
        ];

        let item = extract_observation(
            200,
            "cafe",
            &inputs,
            &outputs,
            &vault, // sender IS the vault
            |a| a == vault,
            |a| a == vault, // protocol-controlled sender
            10_000,
            NET,
        )
        .unwrap()
        .expect("observation");

        // Vault-sent txs observe as a batch outbound (Go
        // getBatchOutboundTxIn): the recipient output, NOT the change —
        // otherwise the chain sees an unmatched vault spend and halts BTC
        // (observed live on thornado-e2e before this was fixed).
        assert_eq!(item.to, recipient);
        assert_eq!(item.coins[0].amount_sats, 30_000_000);
        assert_eq!(item.gas[0].amount_sats, 1_000_000);
        assert_eq!(item.source_inputs.len(), 1);
        assert_eq!(item.source_inputs[0].tx_id, "bb");
        assert_eq!(item.source_inputs[0].amount_sats, 100_000_000);
    }

    #[test]
    fn outbound_batch_aggregates_all_recipients() {
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let r1_hex = p2tr_hex(3);
        let r2_hex = p2tr_hex(4);
        let r1 = p2tr_addr(3);

        let inputs = vec![input("bb", &vault, 100_000_000)];
        let outputs = vec![
            out(0, 30_000_000, &r1_hex),
            out(1, 20_000_000, &r2_hex),
            out(2, 49_000_000, &vault_hex), // change
        ];
        let item = extract_observation(
            200,
            "cafe",
            &inputs,
            &outputs,
            &vault,
            |a| a == vault,
            |a| a == vault,
            10_000,
            NET,
        )
        .unwrap()
        .expect("observation");
        assert_eq!(item.to, r1); // first recipient
        assert_eq!(item.coins[0].amount_sats, 50_000_000); // recipient total
        assert_eq!(item.gas[0].amount_sats, 1_000_000);
    }

    #[test]
    fn pure_consolidation_still_observes_self_output() {
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let inputs = vec![input("bb", &vault, 100_000_000)];
        // ALL outputs back to the vault: consolidation.
        let outputs = vec![out(0, 99_000_000, &vault_hex)];
        let item = extract_observation(
            200,
            "cafe",
            &inputs,
            &outputs,
            &vault,
            |a| a == vault,
            |a| a == vault,
            10_000,
            NET,
        )
        .unwrap()
        .expect("observation");
        assert_eq!(item.to, vault);
        assert_eq!(item.coins[0].amount_sats, 99_000_000);
    }

    #[test]
    fn structural_ignore_returns_none() {
        // empty vin[0] txid -> ignored
        let vault_hex = p2tr_hex(1);
        let inputs = vec![input("", "x", 1_000)];
        let outputs = vec![out(0, 50_000, &vault_hex)];
        let r = extract_observation(
            1, "t", &inputs, &outputs, "s", |_| true, |_| false, 100, NET,
        )
        .unwrap();
        assert!(r.is_none());
    }

    #[test]
    fn no_matching_output_returns_none() {
        // sender not protocol, receiver not a vault -> nothing selected
        let some_hex = p2tr_hex(5);
        let some_addr = p2tr_addr(5);
        let inputs = vec![input("aa", "customer", 60_000_000)];
        let outputs = vec![out(0, 50_000_000, &some_hex)];
        let r = extract_observation(
            1,
            "t",
            &inputs,
            &outputs,
            "customer",
            |_| false, // nothing is a vault
            |_| false,
            10_000,
            NET,
        )
        .unwrap();
        assert!(r.is_none());
        let _ = some_addr;
    }

    #[test]
    fn dust_output_returns_none() {
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let inputs = vec![input("aa", "customer", 60_000)];
        // 5_000 sats output, below a 10_000 dust threshold
        let outputs = vec![out(0, 5_000, &vault_hex)];
        let r = extract_observation(
            1,
            "t",
            &inputs,
            &outputs,
            "customer",
            |a| a == vault,
            |_| false,
            10_000,
            NET,
        )
        .unwrap();
        assert!(r.is_none());
    }

    #[test]
    fn gas_never_underflows() {
        // pathological: outputs exceed inputs (shouldn't happen on-chain, but
        // the sats math must not panic).
        let vault = p2tr_addr(1);
        let vault_hex = p2tr_hex(1);
        let inputs = vec![input("aa", "customer", 10_000)];
        let outputs = vec![out(0, 50_000, &vault_hex)];
        let item = extract_observation(
            1,
            "t",
            &inputs,
            &outputs,
            "customer",
            |a| a == vault,
            |_| false,
            1_000,
            NET,
        )
        .unwrap()
        .expect("observation");
        assert_eq!(item.gas[0].amount_sats, 0);
    }
}
