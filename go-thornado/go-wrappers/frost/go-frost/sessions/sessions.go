package sessions

/*
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../../includes/darwin/arm64 -Wl,-rpath,${SRCDIR}/../../includes/darwin/arm64 -lgofrost
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../../includes/darwin/amd64 -Wl,-rpath,${SRCDIR}/../../includes/darwin/amd64 -lgofrost
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../includes/linux/amd64 -Wl,-rpath,${SRCDIR}/../../includes/linux/amd64 -lgofrost
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../../includes/linux/arm64 -Wl,-rpath,${SRCDIR}/../../includes/linux/arm64 -lgofrost
#include <stdlib.h>
#include <stdint.h>

typedef struct {
	uint8_t* ptr;
	size_t len;
} GoFrostBuf;

int gofrost_keygen_session_new(const uint8_t* ptr, size_t len, int32_t* handle_out, GoFrostBuf* err_out);
int gofrost_sign_session_new(const uint8_t* ptr, size_t len, int32_t* handle_out, GoFrostBuf* err_out);
int gofrost_session_output_message(int32_t handle, GoFrostBuf* out);
int gofrost_session_message_receiver(int32_t handle, const uint8_t* ptr, size_t len, uint32_t index, GoFrostBuf* out);
int gofrost_session_input_message(int32_t handle, const uint8_t* ptr, size_t len, uint32_t* finished_out, GoFrostBuf* err_out);
int gofrost_session_finish(int32_t handle, GoFrostBuf* out);
int gofrost_session_abort_message(int32_t handle, GoFrostBuf* out);
int gofrost_session_free(int32_t handle);
void gofrost_free(void* ptr, size_t len);
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unsafe"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type Keyshare struct {
	Version             uint16   `json:"version"`
	Engine              string   `json:"engine"`
	Participant         string   `json:"participant"`
	Participants        []string `json:"participants"`
	ParticipantIndex    uint16   `json:"participant_index"`
	MinSigners          uint16   `json:"min_signers"`
	MaxSigners          uint16   `json:"max_signers"`
	PublicKeyCompressed string   `json:"public_key_compressed"`
	KeyPackage          string   `json:"key_package"`
	PublicKeyPackage    string   `json:"public_key_package"`
}

type Handle int32

type ffiError struct {
	Code     string `json:"code"`
	Phase    string `json:"phase"`
	Party    uint16 `json:"party"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

type sessionInput struct {
	Participants   []string `json:"participants"`
	Local          string   `json:"local"`
	MinSigners     uint16   `json:"min_signers,omitempty"`
	Share          string   `json:"share,omitempty"`
	Message        string   `json:"message,omitempty"`
	TaprootKeyPath bool     `json:"taproot_key_path,omitempty"`
	MerkleRoot     string   `json:"merkle_root,omitempty"`
	ChildTweak     string   `json:"child_tweak,omitempty"`
}

func KeygenSessionNew(participants []string, local string, minSigners uint16) (Handle, error) {
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoFrostBuf) C.int {
		return C.gofrost_keygen_session_new(ptr, len, handleOut, errOut)
	}, sessionInput{Participants: participants, Local: local, MinSigners: minSigners})
}

func SignSessionNew(participants []string, local string, shareBytes, msg []byte) (Handle, error) {
	return SignSessionNewWithTweak(participants, local, shareBytes, msg, false, nil, nil)
}

func SignSessionNewWithTweak(
	participants []string,
	local string,
	shareBytes, msg []byte,
	taprootKeyPath bool,
	scriptRoot, childTweak []byte,
) (Handle, error) {
	if len(msg) != 32 {
		return 0, fmt.Errorf("FROST messages must be 32 bytes, got %d", len(msg))
	}
	input := sessionInput{
		Participants:   participants,
		Local:          local,
		Share:          base64.StdEncoding.EncodeToString(shareBytes),
		Message:        base64.StdEncoding.EncodeToString(msg),
		TaprootKeyPath: taprootKeyPath,
	}
	if len(scriptRoot) > 0 {
		input.MerkleRoot = base64.StdEncoding.EncodeToString(scriptRoot)
	}
	if len(childTweak) > 0 {
		input.ChildTweak = base64.StdEncoding.EncodeToString(childTweak)
	}
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoFrostBuf) C.int {
		return C.gofrost_sign_session_new(ptr, len, handleOut, errOut)
	}, input)
}

func SessionOutputMessage(handle Handle) ([]byte, error) {
	var out C.GoFrostBuf
	code := C.gofrost_session_output_message(C.int32_t(handle), &out) //nolint:gocritic // cgo wrapper false-positive.
	defer C.gofrost_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return nil, parseError(outBytes)
	}
	if len(outBytes) == 0 {
		return nil, nil
	}
	return outBytes, nil
}

func SessionMessageReceiver(handle Handle, msg []byte, index int) (string, error) {
	var out C.GoFrostBuf
	var ptr *C.uint8_t
	if len(msg) > 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(&msg[0]))
	}
	code := C.gofrost_session_message_receiver(C.int32_t(handle), ptr, C.size_t(len(msg)), C.uint32_t(index), &out) //nolint:gocritic // cgo wrapper false-positive.
	defer C.gofrost_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return "", parseError(outBytes)
	}
	return string(outBytes), nil
}

func SessionInputMessage(handle Handle, msg []byte) (bool, error) {
	var errOut C.GoFrostBuf
	var finished C.uint32_t
	var ptr *C.uint8_t
	if len(msg) > 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(&msg[0]))
	}
	code := C.gofrost_session_input_message(C.int32_t(handle), ptr, C.size_t(len(msg)), &finished, &errOut) //nolint:gocritic // cgo wrapper false-positive.
	defer C.gofrost_free(unsafe.Pointer(errOut.ptr), errOut.len)
	if code != 0 {
		errBytes := C.GoBytes(unsafe.Pointer(errOut.ptr), C.int(errOut.len))
		return false, parseError(errBytes)
	}
	return finished != 0, nil
}

func SessionFinish(handle Handle) ([]byte, error) {
	var out C.GoFrostBuf
	code := C.gofrost_session_finish(C.int32_t(handle), &out) //nolint:gocritic // cgo wrapper false-positive.
	defer C.gofrost_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return nil, parseError(outBytes)
	}
	return outBytes, nil
}

func SessionAbortMessage(handle Handle) ([]byte, error) {
	var out C.GoFrostBuf
	code := C.gofrost_session_abort_message(C.int32_t(handle), &out) //nolint:gocritic // cgo wrapper false-positive.
	defer C.gofrost_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return nil, parseError(outBytes)
	}
	return outBytes, nil
}

func SessionFree(handle Handle) error {
	code := C.gofrost_session_free(C.int32_t(handle))
	if code != 0 {
		return fmt.Errorf("frost session free failed")
	}
	return nil
}

func Verify(pubKeyCompressed, msg, sigBytes []byte) error {
	if len(msg) != 32 {
		return fmt.Errorf("FROST messages must be 32 bytes, got %d", len(msg))
	}
	pub, err := btcec.ParsePubKey(pubKeyCompressed)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("invalid BIP340 signature: %w", err)
	}
	if !sig.Verify(msg, pub) {
		return fmt.Errorf("BIP340 signature verification failed")
	}
	return nil
}

func DecodeKeyshare(buf []byte) (Keyshare, error) {
	var share Keyshare
	if err := json.Unmarshal(buf, &share); err != nil {
		return Keyshare{}, err
	}
	if share.KeyPackage == "" || share.PublicKeyPackage == "" || share.PublicKeyCompressed == "" {
		return Keyshare{}, fmt.Errorf("invalid frost keyshare")
	}
	return share, nil
}

type sessionNewFunc func(*C.uint8_t, C.size_t, *C.int32_t, *C.GoFrostBuf) C.int

func callSessionNew(fn sessionNewFunc, input any) (Handle, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	var errOut C.GoFrostBuf
	var handle C.int32_t
	var inPtr *C.uint8_t
	if len(inputBytes) > 0 {
		inPtr = (*C.uint8_t)(unsafe.Pointer(&inputBytes[0]))
	}
	code := fn(inPtr, C.size_t(len(inputBytes)), &handle, &errOut)
	defer C.gofrost_free(unsafe.Pointer(errOut.ptr), errOut.len)
	if code != 0 {
		errBytes := C.GoBytes(unsafe.Pointer(errOut.ptr), C.int(errOut.len))
		return 0, parseError(errBytes)
	}
	return Handle(handle), nil
}

func parseError(outBytes []byte) error {
	var ferr ffiError
	if err := json.Unmarshal(outBytes, &ferr); err == nil && ferr.Message != "" {
		return fmt.Errorf("%s %s party=%d: %s", ferr.Code, ferr.Phase, ferr.Party, ferr.Message)
	}
	return fmt.Errorf("frost ffi failed")
}