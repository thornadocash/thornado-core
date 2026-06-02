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

int gofrost_keygen(const uint8_t* ptr, size_t len, GoFrostBuf* out);
int gofrost_sign(const uint8_t* ptr, size_t len, GoFrostBuf* out);
int gofrost_sign_taproot_tweak(const uint8_t* ptr, size_t len, GoFrostBuf* out);
int gofrost_verify(const uint8_t* ptr, size_t len, GoFrostBuf* out);
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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unsafe"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type Keyshare struct {
	Version             uint16            `json:"version"`
	Engine              string            `json:"engine"`
	Participant         string            `json:"participant"`
	Participants        []string          `json:"participants"`
	ParticipantIndex    uint16            `json:"participant_index"`
	MinSigners          uint16            `json:"min_signers"`
	MaxSigners          uint16            `json:"max_signers"`
	PublicKeyCompressed string            `json:"public_key_compressed"`
	KeyPackage          string            `json:"key_package"`
	PublicKeyPackage    string            `json:"public_key_package"`
	AllKeyPackages      map[string]string `json:"all_key_packages"`
}

type Handle int32

type ffiError struct {
	Code     string `json:"code"`
	Phase    string `json:"phase"`
	Party    uint16 `json:"party"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

type keygenInput struct {
	Participants []string `json:"participants"`
	MinSigners   uint16   `json:"min_signers,omitempty"`
}

type sessionInput struct {
	Participants []string `json:"participants"`
	Local        string   `json:"local"`
	MinSigners   uint16   `json:"min_signers,omitempty"`
	Share        string   `json:"share,omitempty"`
	Message      string   `json:"message,omitempty"`
}

type keygenOutput struct {
	Shares              map[string]string `json:"shares"`
	PublicKeyCompressed string            `json:"pub_key_compressed"`
}

type signInput struct {
	Share      string  `json:"share"`
	Message    string  `json:"message"`
	MerkleRoot *string `json:"merkle_root,omitempty"`
	ChildTweak *string `json:"child_tweak,omitempty"`
}

type signOutput struct {
	Signature string `json:"signature"`
}

func Keygen(participants []string) (map[string][]byte, []byte, error) {
	return KeygenWithThreshold(participants, 0)
}

func KeygenWithThreshold(participants []string, minSigners uint16) (map[string][]byte, []byte, error) {
	input := keygenInput{Participants: participants, MinSigners: minSigners}
	var output keygenOutput
	if err := callKeygen(input, &output); err != nil {
		return nil, nil, err
	}

	pubKeyBytes, err := hex.DecodeString(output.PublicKeyCompressed)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid FROST public key: %w", err)
	}
	shares := make(map[string][]byte, len(output.Shares))
	for participant, encoded := range output.Shares {
		shareBytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid frost keyshare: %w", err)
		}
		shares[participant] = shareBytes
	}
	return shares, pubKeyBytes, nil
}

func KeygenSessionNew(participants []string, local string, minSigners uint16) (Handle, error) {
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoFrostBuf) C.int {
		return C.gofrost_keygen_session_new(ptr, len, handleOut, errOut)
	}, sessionInput{Participants: participants, Local: local, MinSigners: minSigners})
}

func SignSessionNew(participants []string, local string, shareBytes, msg []byte) (Handle, error) {
	if len(msg) != 32 {
		return 0, fmt.Errorf("FROST messages must be 32 bytes, got %d", len(msg))
	}
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoFrostBuf) C.int {
		return C.gofrost_sign_session_new(ptr, len, handleOut, errOut)
	}, sessionInput{
		Participants: participants,
		Local:        local,
		Share:        base64.StdEncoding.EncodeToString(shareBytes),
		Message:      base64.StdEncoding.EncodeToString(msg),
	})
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

func Sign(shareBytes, msg []byte) ([]byte, error) {
	return signWith(shareBytes, msg, nil, nil)
}

func SignTaprootTweak(shareBytes, msg, merkleRoot []byte) ([]byte, error) {
	return signWith(shareBytes, msg, merkleRoot, nil)
}

func SignTaprootChildTweak(shareBytes, msg, childTweak, merkleRoot []byte) ([]byte, error) {
	return signWith(shareBytes, msg, merkleRoot, childTweak)
}

func signWith(shareBytes, msg, merkleRoot, childTweak []byte) ([]byte, error) {
	if len(msg) != 32 {
		return nil, fmt.Errorf("FROST messages must be 32 bytes, got %d", len(msg))
	}
	input := signInput{
		Share:   base64.StdEncoding.EncodeToString(shareBytes),
		Message: base64.StdEncoding.EncodeToString(msg),
	}
	call := callSign
	if merkleRoot != nil {
		encodedRoot := base64.StdEncoding.EncodeToString(merkleRoot)
		input.MerkleRoot = &encodedRoot
		call = callSignTaprootTweak
	}
	if childTweak != nil {
		encodedTweak := base64.StdEncoding.EncodeToString(childTweak)
		input.ChildTweak = &encodedTweak
		call = callSignTaprootTweak
	}
	var output signOutput
	if err := call(input, &output); err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(output.Signature)
	if err != nil {
		return nil, fmt.Errorf("invalid BIP340 signature: %w", err)
	}
	return sig, nil
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

type cFunc func(*C.uint8_t, C.size_t, *C.GoFrostBuf) C.int

func callKeygen(input any, output any) error {
	return callJSON(func(ptr *C.uint8_t, len C.size_t, out *C.GoFrostBuf) C.int {
		return C.gofrost_keygen(ptr, len, out)
	}, input, output)
}

func callSign(input any, output any) error {
	return callJSON(func(ptr *C.uint8_t, len C.size_t, out *C.GoFrostBuf) C.int {
		return C.gofrost_sign(ptr, len, out)
	}, input, output)
}

func callSignTaprootTweak(input any, output any) error {
	return callJSON(func(ptr *C.uint8_t, len C.size_t, out *C.GoFrostBuf) C.int {
		return C.gofrost_sign_taproot_tweak(ptr, len, out)
	}, input, output)
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

func callJSON(fn cFunc, input any, output any) error {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return err
	}
	var out C.GoFrostBuf
	var inPtr *C.uint8_t
	if len(inputBytes) > 0 {
		inPtr = (*C.uint8_t)(unsafe.Pointer(&inputBytes[0]))
	}
	code := fn(inPtr, C.size_t(len(inputBytes)), &out)
	defer C.gofrost_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return parseError(outBytes)
	}
	if err := json.Unmarshal(outBytes, output); err != nil {
		return fmt.Errorf("invalid frost ffi response: %w", err)
	}
	return nil
}

func parseError(outBytes []byte) error {
	var ferr ffiError
	if err := json.Unmarshal(outBytes, &ferr); err == nil && ferr.Message != "" {
		return fmt.Errorf("%s %s party=%d: %s", ferr.Code, ferr.Phase, ferr.Party, ferr.Message)
	}
	return fmt.Errorf("frost ffi failed")
}
