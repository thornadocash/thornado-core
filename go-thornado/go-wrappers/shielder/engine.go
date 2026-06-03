package shielder

/*
#cgo darwin LDFLAGS: -L${SRCDIR}/../../../target/release -L${SRCDIR}/../../../target/debug -lthornado_ffi -Wl,-rpath,${SRCDIR}/../../../target/release -Wl,-rpath,${SRCDIR}/../../../target/debug
#cgo linux LDFLAGS: -L${SRCDIR}/../../../target/release -L${SRCDIR}/../../../target/debug -lthornado_ffi -Wl,-rpath,${SRCDIR}/../../../target/release -Wl,-rpath,${SRCDIR}/../../../target/debug -ldl -lm -lpthread
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

char* thornado_last_error(void);
void thornado_free_string(char* value);
char* thornado_client_pubkey_from_secret_json(const char* client_seed);
char* thornado_client_pubkey_for_deposit_json(const char* client_seed, uint64_t deposit_index);
char* thornado_client_pubkey_for_deposit_type_json(const char* client_seed, const char* deposit_type, uint64_t deposit_index);
char* thornado_derive_shield_receipt_json(const char* deposit_id, uint64_t amount_sats, const char* client_seed);
char* thornado_derive_shield_receipt_for_deposit_json(const char* deposit_id, uint64_t deposit_index, uint64_t amount_sats, const char* client_seed);
char* thornado_derive_shield_receipt_for_deposit_type_json(const char* deposit_id, const char* deposit_type, uint64_t deposit_index, uint64_t amount_sats, const char* client_seed);
char* thornado_shield_authorization_json(const char* client_seed, const char* deposit_id, uint64_t amount_sats, const char* note_commitments_json);
char* thornado_shield_authorization_for_deposit_json(const char* client_seed, uint64_t deposit_index, const char* deposit_id, uint64_t amount_sats, const char* note_commitments_json);
char* thornado_shield_authorization_for_deposit_type_json(const char* client_seed, const char* deposit_type, uint64_t deposit_index, const char* deposit_id, uint64_t amount_sats, const char* note_commitments_json);
char* thornado_merkle_root_json(const char* leaves_json);
char* thornado_shielder_withdrawal_from_receipt_json(const char* note_json, const char* client_seed, const char* leaves_json, const char* recipient, uint64_t fee_sats);
bool thornado_verify_withdrawal_json(const char* proof_json, const char* public_json);
*/
import "C"

import (
	"errors"
	"unsafe"
)

func LastError() string {
	value := C.thornado_last_error()
	if value == nil {
		return ""
	}
	defer C.thornado_free_string(value)
	return C.GoString(value)
}

func ClientPubKeyFromSecret(clientSeed string) (string, error) {
	seed := cString(clientSeed)
	defer C.free(unsafe.Pointer(seed))
	return takeString(C.thornado_client_pubkey_from_secret_json(seed))
}

func ClientPubKeyForDeposit(clientSeed string, depositIndex uint64) (string, error) {
	seed := cString(clientSeed)
	defer C.free(unsafe.Pointer(seed))
	return takeString(C.thornado_client_pubkey_for_deposit_json(seed, C.uint64_t(depositIndex)))
}

func ClientPubKeyForDepositType(clientSeed string, depositType string, depositIndex uint64) (string, error) {
	seed := cString(clientSeed)
	typ := cString(depositType)
	defer C.free(unsafe.Pointer(seed))
	defer C.free(unsafe.Pointer(typ))
	return takeString(C.thornado_client_pubkey_for_deposit_type_json(seed, typ, C.uint64_t(depositIndex)))
}

func DeriveShieldReceipt(depositID string, amountSats uint64, clientSeed string) (string, error) {
	deposit := cString(depositID)
	seed := cString(clientSeed)
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(seed))
	return takeString(C.thornado_derive_shield_receipt_json(deposit, C.uint64_t(amountSats), seed))
}

func DeriveShieldReceiptForDeposit(depositID string, depositIndex uint64, amountSats uint64, clientSeed string) (string, error) {
	deposit := cString(depositID)
	seed := cString(clientSeed)
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(seed))
	return takeString(C.thornado_derive_shield_receipt_for_deposit_json(deposit, C.uint64_t(depositIndex), C.uint64_t(amountSats), seed))
}

func DeriveShieldReceiptForDepositType(depositID string, depositType string, depositIndex uint64, amountSats uint64, clientSeed string) (string, error) {
	deposit := cString(depositID)
	typ := cString(depositType)
	seed := cString(clientSeed)
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(typ))
	defer C.free(unsafe.Pointer(seed))
	return takeString(C.thornado_derive_shield_receipt_for_deposit_type_json(deposit, typ, C.uint64_t(depositIndex), C.uint64_t(amountSats), seed))
}

func ShieldAuthorization(clientSeed string, depositID string, amountSats uint64, noteCommitmentsJSON string) (string, error) {
	seed := cString(clientSeed)
	deposit := cString(depositID)
	commitments := cString(noteCommitmentsJSON)
	defer C.free(unsafe.Pointer(seed))
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(commitments))
	return takeString(C.thornado_shield_authorization_json(seed, deposit, C.uint64_t(amountSats), commitments))
}

func ShieldAuthorizationForDeposit(clientSeed string, depositIndex uint64, depositID string, amountSats uint64, noteCommitmentsJSON string) (string, error) {
	seed := cString(clientSeed)
	deposit := cString(depositID)
	commitments := cString(noteCommitmentsJSON)
	defer C.free(unsafe.Pointer(seed))
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(commitments))
	return takeString(C.thornado_shield_authorization_for_deposit_json(seed, C.uint64_t(depositIndex), deposit, C.uint64_t(amountSats), commitments))
}

func ShieldAuthorizationForDepositType(clientSeed string, depositType string, depositIndex uint64, depositID string, amountSats uint64, noteCommitmentsJSON string) (string, error) {
	seed := cString(clientSeed)
	typ := cString(depositType)
	deposit := cString(depositID)
	commitments := cString(noteCommitmentsJSON)
	defer C.free(unsafe.Pointer(seed))
	defer C.free(unsafe.Pointer(typ))
	defer C.free(unsafe.Pointer(deposit))
	defer C.free(unsafe.Pointer(commitments))
	return takeString(C.thornado_shield_authorization_for_deposit_type_json(seed, typ, C.uint64_t(depositIndex), deposit, C.uint64_t(amountSats), commitments))
}

func MerkleRoot(leavesJSON string) (string, error) {
	leaves := cString(leavesJSON)
	defer C.free(unsafe.Pointer(leaves))
	return takeString(C.thornado_merkle_root_json(leaves))
}

func ShielderWithdrawalFromReceipt(noteJSON string, clientSeed string, leavesJSON string, recipient string, feeSats uint64) (string, error) {
	note := cString(noteJSON)
	seed := cString(clientSeed)
	leaves := cString(leavesJSON)
	to := cString(recipient)
	defer C.free(unsafe.Pointer(note))
	defer C.free(unsafe.Pointer(seed))
	defer C.free(unsafe.Pointer(leaves))
	defer C.free(unsafe.Pointer(to))
	return takeString(C.thornado_shielder_withdrawal_from_receipt_json(note, seed, leaves, to, C.uint64_t(feeSats)))
}

func VerifyWithdrawal(proofJSON string, publicJSON string) error {
	proof := cString(proofJSON)
	public := cString(publicJSON)
	defer C.free(unsafe.Pointer(proof))
	defer C.free(unsafe.Pointer(public))
	if C.thornado_verify_withdrawal_json(proof, public) {
		return nil
	}
	return lastError()
}

func cString(value string) *C.char {
	return C.CString(value)
}

func takeString(value *C.char) (string, error) {
	if value == nil {
		return "", lastError()
	}
	defer C.thornado_free_string(value)
	return C.GoString(value), nil
}

func lastError() error {
	if message := LastError(); message != "" {
		return errors.New(message)
	}
	return errors.New("thornado privacy engine failed")
}
