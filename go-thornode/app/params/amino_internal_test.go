package params

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestBech32Encoder_UnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	// Pass a non-[]byte value to trigger the default case
	v := protoreflect.ValueOfString("not-bytes")
	err := bech32Encoder(nil, v, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")
}

func TestBech32Encoder_ValidBytes(t *testing.T) {
	var buf bytes.Buffer
	v := protoreflect.ValueOfBytes([]byte{0x01, 0x02, 0x03})
	err := bech32Encoder(nil, v, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, buf.String())
}

func TestAssetEncoder_UnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	// Pass a non-Message value to trigger the first error
	v := protoreflect.ValueOfString("not-a-message")
	err := assetEncoder(nil, v, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported protoreflect message type")
}

func TestKeygenTypeEncoder_UnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	// Pass a non-EnumNumber value to trigger the first error
	v := protoreflect.ValueOfString("not-enum")
	err := keygenTypeEncoder(nil, v, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported protoreflect message type")
}

func TestKeygenTypeEncoder_ValidEnum(t *testing.T) {
	var buf bytes.Buffer
	// Pass a valid keygen type enum (0 = AsgardKeygen)
	v := protoreflect.ValueOfEnum(0)
	err := keygenTypeEncoder(nil, v, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, buf.String())
}

func TestKeygenTypeEncoder_UnknownEnum(t *testing.T) {
	var buf bytes.Buffer
	// Pass an invalid enum number
	v := protoreflect.ValueOfEnum(999)
	err := keygenTypeEncoder(nil, v, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown keygen type")
}
