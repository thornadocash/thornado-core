//! Inbound observation: which transactions and outputs are relevant to our
//! vaults.
//!
//! Ports the pure selection logic from `client_internal.go` — the `ignoreTx`
//! structural pre-filter and `getOutputs` receiver/sender rule — with vault
//! membership supplied by the caller (closures), so the rules are unit-tested
//! without chain state.

/// Maximum outputs a thornado-shaped tx may have (Go `ignoreTx`).
pub const MAX_VALUE_OUTPUTS: usize = 10;

/// A transaction output as seen during scanning.
#[derive(Debug, Clone)]
pub struct Vout {
    pub value: f64,
    /// addresses decoded from the scriptPubKey (0, 1, or many).
    pub addresses: Vec<String>,
    /// scriptPubKey type, e.g. "witness_v1_taproot", "nulldata".
    pub script_type: String,
}

/// A transaction input reference (only the first vin's txid matters to the
/// pre-filter).
#[derive(Debug, Clone)]
pub struct Vin {
    pub txid: String,
}

/// Structural pre-filter (Go `ignoreTx`): ignore txs that can't be a thornado
/// inbound/outbound — no inputs, no/too-many outputs, missing first-input txid,
/// or no output carrying value.
pub fn should_ignore_tx(vins: &[Vin], vouts: &[Vout]) -> bool {
    if vins.is_empty() || vouts.is_empty() || vouts.len() > MAX_VALUE_OUTPUTS {
        return true;
    }
    if vins[0].txid.is_empty() {
        return true;
    }
    let with_value = vouts
        .iter()
        .filter(|v| !v.script_type.eq_ignore_ascii_case("nulldata") && v.value > 0.0)
        .count();
    // no value outputs, or more than the thornado format allows
    with_value == 0 || with_value > MAX_VALUE_OUTPUTS
}

#[derive(Debug, thiserror::Error)]
pub enum ObserveError {
    #[error("no output matched observation criteria")]
    NoOutputMatch,
}

/// Select the observable output indices from a tx (Go `getOutputs`).
///
/// For each value-bearing vout with exactly one address (the receiver):
/// - to be observed, the sender must be protocol-controlled OR the receiver a
///   base (vault) address;
/// - for a consolidate tx, keep outputs where receiver == sender;
/// - otherwise keep outputs where receiver != sender.
///
/// `is_protocol_controlled` and `is_base` are membership predicates supplied by
/// the caller. Returns the selected vout indices, or `NoOutputMatch`.
pub fn select_outputs<P, B>(
    sender: &str,
    vouts: &[Vout],
    consolidate: bool,
    is_protocol_controlled: P,
    is_base: B,
) -> Result<Vec<usize>, ObserveError>
where
    P: Fn(&str) -> bool,
    B: Fn(&str) -> bool,
{
    let sender_protocol = is_protocol_controlled(sender);
    let mut selected = Vec::new();
    for (idx, vout) in vouts.iter().enumerate() {
        if vout.value <= 0.0 {
            continue;
        }
        if vout.addresses.len() != 1 {
            continue; // ambiguous — skip
        }
        let receiver = &vout.addresses[0];
        if !sender_protocol && !is_base(receiver) {
            continue;
        }
        let same = receiver == sender;
        if consolidate && same {
            selected.push(idx);
        } else if !consolidate && !same {
            selected.push(idx);
        }
    }
    if selected.is_empty() {
        return Err(ObserveError::NoOutputMatch);
    }
    Ok(selected)
}

/// The preferred single output (Go `getOutput` = first of `getOutputs`).
pub fn select_output<P, B>(
    sender: &str,
    vouts: &[Vout],
    consolidate: bool,
    is_protocol_controlled: P,
    is_base: B,
) -> Result<usize, ObserveError>
where
    P: Fn(&str) -> bool,
    B: Fn(&str) -> bool,
{
    select_outputs(sender, vouts, consolidate, is_protocol_controlled, is_base)
        .map(|v| v[0])
}

#[cfg(test)]
mod tests {
    use super::*;

    fn vout(value: f64, addr: &str) -> Vout {
        Vout {
            value,
            addresses: if addr.is_empty() { vec![] } else { vec![addr.to_string()] },
            script_type: "witness_v1_taproot".into(),
        }
    }
    fn vin(txid: &str) -> Vin {
        Vin { txid: txid.into() }
    }

    #[test]
    fn ignore_structural_cases() {
        assert!(should_ignore_tx(&[], &[vout(1.0, "a")])); // no vins
        assert!(should_ignore_tx(&[vin("t")], &[])); // no vouts
        assert!(should_ignore_tx(&[vin("")], &[vout(1.0, "a")])); // empty first vin txid
        // 11 outputs -> too many
        let many: Vec<Vout> = (0..11).map(|_| vout(1.0, "a")).collect();
        assert!(should_ignore_tx(&[vin("t")], &many));
        // all zero-value
        assert!(should_ignore_tx(&[vin("t")], &[vout(0.0, "a"), vout(0.0, "b")]));
    }

    #[test]
    fn ignore_nulldata_only_still_ignored() {
        let mut op_return = vout(0.0, "");
        op_return.script_type = "nulldata".into();
        assert!(should_ignore_tx(&[vin("t")], &[op_return]));
    }

    #[test]
    fn keeps_normal_inbound() {
        assert!(!should_ignore_tx(&[vin("t")], &[vout(0.5, "vault"), vout(0.1, "change")]));
    }

    #[test]
    fn inbound_selects_output_to_vault() {
        // sender is a customer (not protocol), receiver "vault" is a base addr.
        let vouts = vec![vout(0.5, "vault"), vout(0.2, "customer_change")];
        let sel = select_outputs(
            "customer",
            &vouts,
            false,
            |_| false,             // sender not protocol-controlled
            |a| a == "vault",      // "vault" is base
        )
        .unwrap();
        assert_eq!(sel, vec![0]); // only the vault-bound output
    }

    #[test]
    fn outbound_from_vault_selects_recipient_not_change() {
        // sender IS a vault (protocol-controlled); pick outputs to others,
        // skip change back to self.
        let vouts = vec![vout(0.3, "recipient"), vout(0.6, "vault_self")];
        let sel = select_outputs(
            "vault_self",
            &vouts,
            false,
            |a| a == "vault_self", // sender protocol-controlled
            |_| false,
        )
        .unwrap();
        assert_eq!(sel, vec![0]); // recipient, not the self-change
    }

    #[test]
    fn consolidate_selects_self_output() {
        let vouts = vec![vout(0.9, "vault_self")];
        let sel = select_outputs(
            "vault_self",
            &vouts,
            true, // consolidate
            |a| a == "vault_self",
            |_| false,
        )
        .unwrap();
        assert_eq!(sel, vec![0]);
    }

    #[test]
    fn no_match_errors() {
        // multi-address vout is skipped; nothing left
        let ambiguous = Vout {
            value: 1.0,
            addresses: vec!["a".into(), "b".into()],
            script_type: "multisig".into(),
        };
        let r = select_outputs("s", &[ambiguous], false, |_| true, |_| true);
        assert!(matches!(r, Err(ObserveError::NoOutputMatch)));
    }
}
