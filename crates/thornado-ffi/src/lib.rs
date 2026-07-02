use std::cell::RefCell;
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

use serde::Serialize;
use thornado_shielder::{
    client_pubkey_for_deposit, client_pubkey_for_deposit_type, client_pubkey_from_secret,
    derive_shield_receipt, derive_shield_receipt_for_deposit,
    derive_shield_receipt_for_deposit_type, merkle_append, merkle_root, recipient_binding_field,
    shield_authorization, shield_authorization_for_deposit, shield_authorization_for_deposit_type,
    shielder_withdrawal_from_receipt, validate_withdrawal_public_inputs, DenominationTree,
    NoteCommitment, NoteReceipt, ShielderProofVerifier, WithdrawalProof, WithdrawalPublicInputs,
};

thread_local! {
    static LAST_ERROR: RefCell<Option<String>> = RefCell::new(None);
}

/// Runs an FFI entry-point body under `catch_unwind` so a Rust panic can never
/// unwind across the C ABI (which would be undefined behavior). A caught panic is
/// routed to `fallback`, mirroring the normal error path of each entry point.
fn guard_ffi<T>(fallback: T, body: impl FnOnce() -> T + std::panic::UnwindSafe) -> T {
    match std::panic::catch_unwind(body) {
        Ok(value) => value,
        Err(payload) => {
            let message = panic_message(&payload);
            set_error(format!("panic: {message}"));
            fallback
        }
    }
}

fn panic_message(payload: &Box<dyn std::any::Any + Send>) -> String {
    if let Some(message) = payload.downcast_ref::<&str>() {
        (*message).to_string()
    } else if let Some(message) = payload.downcast_ref::<String>() {
        message.clone()
    } else {
        "unknown panic".to_string()
    }
}

#[no_mangle]
pub extern "C" fn thornado_free_string(value: *mut c_char) {
    if value.is_null() {
        return;
    }
    unsafe {
        drop(CString::from_raw(value));
    }
}

#[no_mangle]
pub extern "C" fn thornado_last_error() -> *mut c_char {
    guard_ffi(std::ptr::null_mut(), || {
        LAST_ERROR.with(|slot| match slot.borrow().as_ref() {
            Some(message) => into_c_string(message),
            None => std::ptr::null_mut(),
        })
    })
}

#[no_mangle]
pub extern "C" fn thornado_client_pubkey_from_secret_json(
    client_seed: *const c_char,
) -> *mut c_char {
    return_string_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        Ok(client_pubkey_from_secret(client_seed))
    })
}

#[no_mangle]
pub extern "C" fn thornado_client_pubkey_for_deposit_json(
    client_seed: *const c_char,
    deposit_index: u64,
) -> *mut c_char {
    return_string_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        Ok(client_pubkey_for_deposit(client_seed, deposit_index))
    })
}

#[no_mangle]
pub extern "C" fn thornado_client_pubkey_for_deposit_type_json(
    client_seed: *const c_char,
    deposit_type: *const c_char,
    deposit_index: u64,
) -> *mut c_char {
    return_string_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        let deposit_type = c_str(deposit_type, "deposit_type")?;
        Ok(client_pubkey_for_deposit_type(
            client_seed,
            deposit_type,
            deposit_index,
        ))
    })
}

#[no_mangle]
pub extern "C" fn thornado_derive_shield_receipt_json(
    deposit_id: *const c_char,
    amount_sats: u64,
    client_seed: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let client_seed = c_str(client_seed, "client_seed")?;
        derive_shield_receipt(deposit_id, amount_sats, client_seed)
            .map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_derive_shield_receipt_for_deposit_json(
    deposit_id: *const c_char,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let client_seed = c_str(client_seed, "client_seed")?;
        derive_shield_receipt_for_deposit(deposit_id, deposit_index, amount_sats, client_seed)
            .map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_derive_shield_receipt_for_deposit_type_json(
    deposit_id: *const c_char,
    deposit_type: *const c_char,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let deposit_type = c_str(deposit_type, "deposit_type")?;
        let client_seed = c_str(client_seed, "client_seed")?;
        derive_shield_receipt_for_deposit_type(
            deposit_id,
            deposit_type,
            deposit_index,
            amount_sats,
            client_seed,
        )
        .map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_shield_authorization_json(
    client_seed: *const c_char,
    deposit_id: *const c_char,
    amount_sats: u64,
    note_commitments_json: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let note_commitments_json = c_str(note_commitments_json, "note_commitments_json")?;
        let note_commitments: Vec<NoteCommitment> =
            serde_json::from_str(note_commitments_json).map_err(|error| error.to_string())?;
        Ok(shield_authorization(
            client_seed,
            deposit_id,
            amount_sats,
            &note_commitments,
        ))
    })
}

#[no_mangle]
pub extern "C" fn thornado_shield_authorization_for_deposit_json(
    client_seed: *const c_char,
    deposit_index: u64,
    deposit_id: *const c_char,
    amount_sats: u64,
    note_commitments_json: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let note_commitments_json = c_str(note_commitments_json, "note_commitments_json")?;
        let note_commitments: Vec<NoteCommitment> =
            serde_json::from_str(note_commitments_json).map_err(|error| error.to_string())?;
        Ok(shield_authorization_for_deposit(
            client_seed,
            deposit_index,
            deposit_id,
            amount_sats,
            &note_commitments,
        ))
    })
}

#[no_mangle]
pub extern "C" fn thornado_shield_authorization_for_deposit_type_json(
    client_seed: *const c_char,
    deposit_type: *const c_char,
    deposit_index: u64,
    deposit_id: *const c_char,
    amount_sats: u64,
    note_commitments_json: *const c_char,
) -> *mut c_char {
    return_json_result(|| {
        let client_seed = c_str(client_seed, "client_seed")?;
        let deposit_type = c_str(deposit_type, "deposit_type")?;
        let deposit_id = c_str(deposit_id, "deposit_id")?;
        let note_commitments_json = c_str(note_commitments_json, "note_commitments_json")?;
        let note_commitments: Vec<NoteCommitment> =
            serde_json::from_str(note_commitments_json).map_err(|error| error.to_string())?;
        Ok(shield_authorization_for_deposit_type(
            client_seed,
            deposit_type,
            deposit_index,
            deposit_id,
            amount_sats,
            &note_commitments,
        ))
    })
}

#[no_mangle]
pub extern "C" fn thornado_merkle_root_json(leaves_json: *const c_char) -> *mut c_char {
    return_string_result(|| {
        let leaves_json = c_str(leaves_json, "leaves_json")?;
        let leaves: Vec<String> =
            serde_json::from_str(leaves_json).map_err(|error| error.to_string())?;
        Ok(merkle_root(&leaves))
    })
}

#[no_mangle]
pub extern "C" fn thornado_shielder_merkle_append_json(request_json: *const c_char) -> *mut c_char {
    return_json_result(|| {
        let request_json = c_str(request_json, "request_json")?;
        let request: thornado_shielder::MerkleAppendRequest =
            serde_json::from_str(request_json).map_err(|error| error.to_string())?;
        merkle_append(&request).map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_shielder_withdrawal_from_receipt_json(
    note_json: *const c_char,
    client_seed: *const c_char,
    leaves_json: *const c_char,
    recipient: *const c_char,
    fee_sats: u64,
) -> *mut c_char {
    return_json_result(|| {
        let note_json = c_str(note_json, "note_json")?;
        let client_seed = c_str(client_seed, "client_seed")?;
        let leaves_json = c_str(leaves_json, "leaves_json")?;
        let recipient = c_str(recipient, "recipient")?;
        let note: NoteReceipt =
            serde_json::from_str(note_json).map_err(|error| error.to_string())?;
        let leaves: Vec<String> =
            serde_json::from_str(leaves_json).map_err(|error| error.to_string())?;
        let tree = DenominationTree {
            leaves,
            known_roots: Default::default(),
        };
        shielder_withdrawal_from_receipt(&note, client_seed, &tree, recipient.to_string(), fee_sats)
            .map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_recipient_binding_field_json(
    recipient: *const c_char,
    fee_sats: u64,
    denomination_sats: u64,
) -> *mut c_char {
    return_string_result(|| {
        let recipient = c_str(recipient, "recipient")?;
        recipient_binding_field(recipient, fee_sats, denomination_sats)
            .map_err(|error| error.to_string())
    })
}

#[no_mangle]
pub extern "C" fn thornado_validate_withdrawal_public_json(public_json: *const c_char) -> bool {
    guard_ffi(
        false,
        std::panic::AssertUnwindSafe(|| {
            match (|| {
                let public_json = c_str(public_json, "public_json")?;
                let public: WithdrawalPublicInputs =
                    serde_json::from_str(public_json).map_err(|error| error.to_string())?;
                validate_withdrawal_public_inputs(&public).map_err(|error| error.to_string())
            })() {
                Ok(()) => {
                    clear_error();
                    true
                }
                Err(error) => {
                    set_error(error);
                    false
                }
            }
        }),
    )
}

#[no_mangle]
pub extern "C" fn thornado_verify_withdrawal_json(
    proof_json: *const c_char,
    public_json: *const c_char,
) -> bool {
    guard_ffi(
        false,
        std::panic::AssertUnwindSafe(|| {
            match (|| {
                let proof_json = c_str(proof_json, "proof_json")?;
                let public_json = c_str(public_json, "public_json")?;
                let proof: WithdrawalProof =
                    serde_json::from_str(proof_json).map_err(|error| error.to_string())?;
                let public: WithdrawalPublicInputs =
                    serde_json::from_str(public_json).map_err(|error| error.to_string())?;
                ShielderProofVerifier
                    .verify_withdrawal(&proof, &public)
                    .map_err(|error| error.to_string())
            })() {
                Ok(()) => {
                    clear_error();
                    true
                }
                Err(error) => {
                    set_error(error);
                    false
                }
            }
        }),
    )
}

fn return_json_result<T: Serialize>(body: impl FnOnce() -> Result<T, String>) -> *mut c_char {
    return_string_result(|| {
        let value = body()?;
        serde_json::to_string(&value).map_err(|error| error.to_string())
    })
}

fn return_string_result(body: impl FnOnce() -> Result<String, String>) -> *mut c_char {
    guard_ffi(
        std::ptr::null_mut(),
        std::panic::AssertUnwindSafe(|| match body() {
            Ok(value) => {
                clear_error();
                into_c_string(&value)
            }
            Err(error) => {
                set_error(error);
                std::ptr::null_mut()
            }
        }),
    )
}

fn c_str<'a>(value: *const c_char, name: &str) -> Result<&'a str, String> {
    if value.is_null() {
        return Err(format!("{name} is null"));
    }
    unsafe { CStr::from_ptr(value) }
        .to_str()
        .map_err(|error| format!("{name} is not UTF-8: {error}"))
}

fn into_c_string(value: &str) -> *mut c_char {
    let sanitized = value.replace('\0', "\\0");
    CString::new(sanitized)
        .expect("sanitized string has no interior nul")
        .into_raw()
}

fn set_error(error: String) {
    LAST_ERROR.with(|slot| {
        *slot.borrow_mut() = Some(error);
    });
}

fn clear_error() {
    LAST_ERROR.with(|slot| {
        *slot.borrow_mut() = None;
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;

    fn c_string(value: &str) -> CString {
        CString::new(value).unwrap()
    }

    fn take_string(value: *mut c_char) -> String {
        assert!(!value.is_null());
        unsafe {
            let string = CStr::from_ptr(value).to_str().unwrap().to_string();
            thornado_free_string(value);
            string
        }
    }

    #[test]
    fn reports_null_input_as_last_error() {
        let value = thornado_client_pubkey_from_secret_json(std::ptr::null());
        assert!(value.is_null());
        let error = take_string(thornado_last_error());
        assert_eq!(error, "client_seed is null");
    }

    #[test]
    fn derives_receipt_and_authorization_through_c_abi() {
        let deposit_id = c_string("dep-1");
        let seed = c_string("client-seed");
        let receipt_json = take_string(thornado_derive_shield_receipt_json(
            deposit_id.as_ptr(),
            100_000_000,
            seed.as_ptr(),
        ));
        let receipt: Value = serde_json::from_str(&receipt_json).unwrap();
        assert_eq!(receipt["notes"][0]["denomination_sats"], 100_000_000);

        let commitments = serde_json::json!([{
            "denomination_sats": receipt["notes"][0]["denomination_sats"],
            "commitment": receipt["notes"][0]["commitment"],
        }])
        .to_string();
        let commitments = c_string(&commitments);
        let authorization_json = take_string(thornado_shield_authorization_json(
            seed.as_ptr(),
            deposit_id.as_ptr(),
            100_000_000,
            commitments.as_ptr(),
        ));
        let authorization: Value = serde_json::from_str(&authorization_json).unwrap();
        assert!(authorization["deposit_pubkey"].as_str().unwrap().len() > 60);
        assert!(authorization["signature"].as_str().unwrap().len() > 60);
    }

    #[test]
    fn withdrawal_payload_without_groth16_is_rejected() {
        let public_json = c_string(
            r#"{"nullifier_hash":"1","merkle_root":"2","denomination_sats":100000,"recipient":"bcrt1qrecipient","fee_sats":1000}"#,
        );
        assert!(thornado_validate_withdrawal_public_json(
            public_json.as_ptr()
        ));
        let proof_json =
            c_string(r#"{"merkle_root":"2","tornado":{"protocol":"tornado-cash-groth16-v2.1"}}"#);
        assert!(!thornado_verify_withdrawal_json(
            proof_json.as_ptr(),
            public_json.as_ptr(),
        ));
        let error = take_string(thornado_last_error());
        assert!(!error.is_empty());
    }

    #[test]
    fn off_curve_proof_point_returns_error_not_panic() {
        let public_json = c_string(
            r#"{"nullifier_hash":"1","merkle_root":"2","denomination_sats":100000,"recipient":"bcrt1qrecipient","fee_sats":1000}"#,
        );
        // pi_a = ["1","1"] is an off-curve bn254 G1 point. Before the fix this panicked
        // inside `G1Affine::new`, unwinding across the C ABI. It must now return false.
        let proof_json = c_string(
            r#"{"merkle_root":"2","tornado":{"protocol":"tornado-cash-groth16-v2.1","groth16":{"pi_a":["1","1"],"pi_b":[["1","1"],["1","1"]],"pi_c":["1","1"],"protocol":"groth"}}}"#,
        );
        let verified = thornado_verify_withdrawal_json(proof_json.as_ptr(), public_json.as_ptr());
        assert!(!verified);
        let error = take_string(thornado_last_error());
        assert!(!error.is_empty());
    }

    #[test]
    #[cfg_attr(
        not(feature = "proof-tests"),
        ignore = "requires production proving key; run with `cargo test -p thornado-ffi --features proof-tests` after `npm run download-artifacts`"
    )]
    fn proves_and_verifies_withdrawal_through_c_abi() {
        let deposit_id = c_string("dep-1");
        let seed = c_string("client-seed");
        let receipt_json = take_string(thornado_derive_shield_receipt_json(
            deposit_id.as_ptr(),
            100_000_000,
            seed.as_ptr(),
        ));
        let receipt: Value = serde_json::from_str(&receipt_json).unwrap();
        let note_json = receipt["notes"][0].to_string();
        let leaves_json = serde_json::json!([receipt["notes"][0]["commitment"]]).to_string();
        let note_json = c_string(&note_json);
        let leaves_json = c_string(&leaves_json);
        let recipient = c_string("bcrt1qrecipient");

        let withdrawal_json = take_string(thornado_shielder_withdrawal_from_receipt_json(
            note_json.as_ptr(),
            seed.as_ptr(),
            leaves_json.as_ptr(),
            recipient.as_ptr(),
            1_000,
        ));
        let withdrawal: (WithdrawalProof, WithdrawalPublicInputs) =
            serde_json::from_str(&withdrawal_json).unwrap();
        let proof_json = c_string(&serde_json::to_string(&withdrawal.0).unwrap());
        let public_json = c_string(&serde_json::to_string(&withdrawal.1).unwrap());

        assert!(thornado_verify_withdrawal_json(
            proof_json.as_ptr(),
            public_json.as_ptr(),
        ));
    }
}
