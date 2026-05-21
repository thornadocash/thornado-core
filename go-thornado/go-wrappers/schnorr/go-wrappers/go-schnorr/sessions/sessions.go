package sessions

/*
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../../includes/darwin/arm64 -Wl,-rpath,${SRCDIR}/../../includes/darwin/arm64 -lgoschnorr
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../../includes/darwin/amd64 -Wl,-rpath,${SRCDIR}/../../includes/darwin/amd64 -lgoschnorr
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../includes/linux/amd64 -Wl,-rpath,${SRCDIR}/../../includes/linux/amd64 -lgoschnorr
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../../includes/linux/arm64 -Wl,-rpath,${SRCDIR}/../../includes/linux/arm64 -lgoschnorr
#include <stdlib.h>
#include <stdint.h>

typedef struct {
	uint8_t* ptr;
	size_t len;
} GoSchnorrBuf;

int goschnorr_keygen(const uint8_t* ptr, size_t len, GoSchnorrBuf* out);
int goschnorr_sign(const uint8_t* ptr, size_t len, GoSchnorrBuf* out);
int goschnorr_verify(const uint8_t* ptr, size_t len, GoSchnorrBuf* out);
int goschnorr_keygen_session_new(const uint8_t* ptr, size_t len, int32_t* handle_out, GoSchnorrBuf* err_out);
int goschnorr_sign_session_new(const uint8_t* ptr, size_t len, int32_t* handle_out, GoSchnorrBuf* err_out);
int goschnorr_session_output_message(int32_t handle, GoSchnorrBuf* out);
int goschnorr_session_message_receiver(int32_t handle, const uint8_t* ptr, size_t len, uint32_t index, GoSchnorrBuf* out);
int goschnorr_session_input_message(int32_t handle, const uint8_t* ptr, size_t len, uint32_t* finished_out, GoSchnorrBuf* err_out);
int goschnorr_session_finish(int32_t handle, GoSchnorrBuf* out);
int goschnorr_session_abort_message(int32_t handle, GoSchnorrBuf* out);
int goschnorr_session_free(int32_t handle);
void goschnorr_free(void* ptr, size_t len);
*/
import "C"

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unsafe"

	btcec "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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
	PathIndex    uint64   `json:"path_index,omitempty"`
}

type keygenOutput struct {
	Shares              map[string]string `json:"shares"`
	PublicKeyCompressed string            `json:"pub_key_compressed"`
}

type signInput struct {
	Share     string `json:"share"`
	Message   string `json:"message"`
	PathIndex uint64 `json:"path_index,omitempty"`
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
		return nil, nil, fmt.Errorf("invalid schnorr public key: %w", err)
	}
	shares := make(map[string][]byte, len(output.Shares))
	for participant, encoded := range output.Shares {
		shareBytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid schnorr keyshare: %w", err)
		}
		shares[participant] = shareBytes
	}
	return shares, pubKeyBytes, nil
}

func KeygenSessionNew(participants []string, local string, minSigners uint16) (Handle, error) {
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoSchnorrBuf) C.int {
		return C.goschnorr_keygen_session_new(ptr, len, handleOut, errOut)
	}, sessionInput{Participants: participants, Local: local, MinSigners: minSigners})
}

func SignSessionNew(participants []string, local string, shareBytes, msg []byte) (Handle, error) {
	return SignSessionNewWithPath(participants, local, shareBytes, msg, 0)
}

func SignSessionNewWithPath(participants []string, local string, shareBytes, msg []byte, pathIndex uint64) (Handle, error) {
	if len(msg) != 32 {
		return 0, fmt.Errorf("schnorr messages must be 32 bytes, got %d", len(msg))
	}
	return callSessionNew(func(ptr *C.uint8_t, len C.size_t, handleOut *C.int32_t, errOut *C.GoSchnorrBuf) C.int {
		return C.goschnorr_sign_session_new(ptr, len, handleOut, errOut)
	}, sessionInput{
		Participants: participants,
		Local:        local,
		Share:        base64.StdEncoding.EncodeToString(shareBytes),
		Message:      base64.StdEncoding.EncodeToString(msg),
		PathIndex:    pathIndex,
	})
}

func SessionOutputMessage(handle Handle) ([]byte, error) {
	var out C.GoSchnorrBuf
	code := C.goschnorr_session_output_message(C.int32_t(handle), &out)
	defer C.goschnorr_free(unsafe.Pointer(out.ptr), out.len)
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
	var out C.GoSchnorrBuf
	var ptr *C.uint8_t
	if len(msg) > 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(&msg[0]))
	}
	code := C.goschnorr_session_message_receiver(C.int32_t(handle), ptr, C.size_t(len(msg)), C.uint32_t(index), &out)
	defer C.goschnorr_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return "", parseError(outBytes)
	}
	return string(outBytes), nil
}

func SessionInputMessage(handle Handle, msg []byte) (bool, error) {
	var errOut C.GoSchnorrBuf
	var finished C.uint32_t
	var ptr *C.uint8_t
	if len(msg) > 0 {
		ptr = (*C.uint8_t)(unsafe.Pointer(&msg[0]))
	}
	code := C.goschnorr_session_input_message(C.int32_t(handle), ptr, C.size_t(len(msg)), &finished, &errOut)
	defer C.goschnorr_free(unsafe.Pointer(errOut.ptr), errOut.len)
	if code != 0 {
		errBytes := C.GoBytes(unsafe.Pointer(errOut.ptr), C.int(errOut.len))
		return false, parseError(errBytes)
	}
	return finished != 0, nil
}

func SessionFinish(handle Handle) ([]byte, error) {
	var out C.GoSchnorrBuf
	code := C.goschnorr_session_finish(C.int32_t(handle), &out)
	defer C.goschnorr_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return nil, parseError(outBytes)
	}
	return outBytes, nil
}

func SessionAbortMessage(handle Handle) ([]byte, error) {
	var out C.GoSchnorrBuf
	code := C.goschnorr_session_abort_message(C.int32_t(handle), &out)
	defer C.goschnorr_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return nil, parseError(outBytes)
	}
	return outBytes, nil
}

func SessionFree(handle Handle) error {
	code := C.goschnorr_session_free(C.int32_t(handle))
	if code != 0 {
		return fmt.Errorf("schnorr session free failed")
	}
	return nil
}

func Sign(shareBytes, msg []byte) ([]byte, error) {
	return SignWithPath(shareBytes, msg, 0)
}

func SignWithPath(shareBytes, msg []byte, pathIndex uint64) ([]byte, error) {
	if len(msg) != 32 {
		return nil, fmt.Errorf("schnorr messages must be 32 bytes, got %d", len(msg))
	}
	input := signInput{
		Share:     base64.StdEncoding.EncodeToString(shareBytes),
		Message:   base64.StdEncoding.EncodeToString(msg),
		PathIndex: pathIndex,
	}
	var output signOutput
	if err := callSign(input, &output); err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(output.Signature)
	if err != nil {
		return nil, fmt.Errorf("invalid schnorr signature: %w", err)
	}
	return sig, nil
}

func DerivePublicKey(pubKeyCompressed []byte, pathIndex uint64) ([]byte, error) {
	pub, err := btcec.ParsePubKey(pubKeyCompressed)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	if pathIndex == 0 {
		return schnorr.SerializePubKey(pub), nil
	}
	var index [8]byte
	binary.BigEndian.PutUint64(index[:], pathIndex)
	h := sha256.New()
	h.Write([]byte("thornado:vault-path:v1"))
	h.Write(pubKeyCompressed)
	h.Write(index[:])
	tweaked := taprootOutputKey(pub, h.Sum(nil))
	return schnorr.SerializePubKey(tweaked), nil
}

func taprootOutputKey(pubKey *btcec.PublicKey, scriptRoot []byte) *btcec.PublicKey {
	internalKey, _ := schnorr.ParsePubKey(schnorr.SerializePubKey(pubKey))
	tapTweakHash := chainhash.TaggedHash(chainhash.TagTapTweak, schnorr.SerializePubKey(internalKey), scriptRoot)

	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(tapTweakHash))

	var internalPoint btcec.JacobianPoint
	internalKey.AsJacobian(&internalPoint)

	var tPoint, taprootKey btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&tweakScalar, &tPoint)
	btcec.AddNonConst(&internalPoint, &tPoint, &taprootKey)
	taprootKey.ToAffine()

	return btcec.NewPublicKey(&taprootKey.X, &taprootKey.Y)
}

func Verify(pubKeyCompressed, msg, sigBytes []byte) error {
	if len(msg) != 32 {
		return fmt.Errorf("schnorr messages must be 32 bytes, got %d", len(msg))
	}
	var pub *btcec.PublicKey
	var err error
	if len(pubKeyCompressed) == 32 {
		pub, err = schnorr.ParsePubKey(pubKeyCompressed)
	} else {
		pub, err = btcec.ParsePubKey(pubKeyCompressed)
	}
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("invalid schnorr signature: %w", err)
	}
	if !sig.Verify(msg, pub) {
		return fmt.Errorf("schnorr signature verification failed")
	}
	return nil
}

func DecodeKeyshare(buf []byte) (Keyshare, error) {
	var share Keyshare
	if err := json.Unmarshal(buf, &share); err != nil {
		return Keyshare{}, err
	}
	if share.KeyPackage == "" || share.PublicKeyPackage == "" || share.PublicKeyCompressed == "" {
		return Keyshare{}, fmt.Errorf("invalid schnorr keyshare")
	}
	return share, nil
}

type cFunc func(*C.uint8_t, C.size_t, *C.GoSchnorrBuf) C.int

func callKeygen(input any, output any) error {
	return callJSON(func(ptr *C.uint8_t, len C.size_t, out *C.GoSchnorrBuf) C.int {
		return C.goschnorr_keygen(ptr, len, out)
	}, input, output)
}

func callSign(input any, output any) error {
	return callJSON(func(ptr *C.uint8_t, len C.size_t, out *C.GoSchnorrBuf) C.int {
		return C.goschnorr_sign(ptr, len, out)
	}, input, output)
}

type sessionNewFunc func(*C.uint8_t, C.size_t, *C.int32_t, *C.GoSchnorrBuf) C.int

func callSessionNew(fn sessionNewFunc, input any) (Handle, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	var errOut C.GoSchnorrBuf
	var handle C.int32_t
	var inPtr *C.uint8_t
	if len(inputBytes) > 0 {
		inPtr = (*C.uint8_t)(unsafe.Pointer(&inputBytes[0]))
	}
	code := fn(inPtr, C.size_t(len(inputBytes)), &handle, &errOut)
	defer C.goschnorr_free(unsafe.Pointer(errOut.ptr), errOut.len)
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
	var out C.GoSchnorrBuf
	var inPtr *C.uint8_t
	if len(inputBytes) > 0 {
		inPtr = (*C.uint8_t)(unsafe.Pointer(&inputBytes[0]))
	}
	code := fn(inPtr, C.size_t(len(inputBytes)), &out)
	defer C.goschnorr_free(unsafe.Pointer(out.ptr), out.len)
	outBytes := C.GoBytes(unsafe.Pointer(out.ptr), C.int(out.len))
	if code != 0 {
		return parseError(outBytes)
	}
	if err := json.Unmarshal(outBytes, output); err != nil {
		return fmt.Errorf("invalid schnorr ffi response: %w", err)
	}
	return nil
}

func parseError(outBytes []byte) error {
	var ferr ffiError
	if err := json.Unmarshal(outBytes, &ferr); err == nil && ferr.Message != "" {
		return fmt.Errorf("%s %s party=%d: %s", ferr.Code, ferr.Phase, ferr.Party, ferr.Message)
	}
	return fmt.Errorf("schnorr ffi failed")
}
