package types

import (
	"errors"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	"github.com/thornadocash/go-thornado/common"
)

func NewVolumeBucket(asset common.Asset, index int64) VolumeBucket {
	return VolumeBucket{
		Asset:       asset,
		Index:       index,
		AmountRune:  math.ZeroUint(),
		AmountAsset: math.ZeroUint(),
	}
}

func (m *VolumeBucket) Valid() error {
	if m.Asset.IsEmpty() {
		return errors.New("asset is empty")
	}

	if m.AmountRune.IsNil() {
		return errors.New("amount rune is empty")
	}

	if m.AmountAsset.IsNil() {
		return errors.New("amount asset is empty")
	}

	if m.Index < 0 {
		return errors.New("index is less than zero")
	}

	return nil
}

// String implement stringer interface
func (m VolumeBucket) String() string {
	sb := strings.Builder{}
	sb.WriteString("asset:" + m.Asset.String())
	sb.WriteString(" index:" + strconv.FormatInt(m.Index, 10))
	sb.WriteString(" amount-rune:" + m.AmountRune.String())
	sb.WriteString(" amount-asset:" + m.AmountAsset.String())
	return sb.String()
}

func (m VolumeBucket) Equals(other VolumeBucket) bool {
	return m.Asset.Equals(other.Asset) &&
		m.Index == other.Index &&
		m.AmountRune.Equal(other.AmountRune) &&
		m.AmountAsset.Equal(other.AmountAsset)
}
