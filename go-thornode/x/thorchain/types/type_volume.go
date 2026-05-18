package types

import (
	"errors"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	"gitlab.com/thorchain/thornode/v3/common"
)

func NewVolume(asset common.Asset) Volume {
	return Volume{
		Asset:       asset.GetLayer1Asset(),
		TotalRune:   math.ZeroUint(),
		TotalAsset:  math.ZeroUint(),
		ChangeRune:  math.ZeroUint(),
		ChangeAsset: math.ZeroUint(),
		LastBucket:  -1,
	}
}

func (m *Volume) Valid() error {
	if m.Asset.IsEmpty() {
		return errors.New("asset is empty")
	}

	if m.TotalRune.IsNil() {
		return errors.New("total rune is empty")
	}

	if m.TotalAsset.IsNil() {
		return errors.New("total asset is empty")
	}

	if m.ChangeRune.IsNil() {
		return errors.New("change rune is empty")
	}

	if m.ChangeAsset.IsNil() {
		return errors.New("change asset is empty")
	}

	return nil
}

// String implement stringer interface
func (m Volume) String() string {
	sb := strings.Builder{}
	sb.WriteString("asset:" + m.Asset.String())
	sb.WriteString(" total-rune:" + m.TotalRune.String())
	sb.WriteString(" total-asset:" + m.TotalAsset.String())
	sb.WriteString(" change-rune:" + m.ChangeRune.String())
	sb.WriteString(" change-asset:" + m.ChangeAsset.String())
	sb.WriteString(" last-bucket:" + strconv.FormatInt(m.LastBucket, 10))
	return sb.String()
}

func (m Volume) Equals(other Volume) bool {
	return m.Asset.Equals(other.Asset) &&
		m.TotalRune.Equal(other.TotalRune) &&
		m.TotalAsset.Equal(other.TotalAsset) &&
		m.ChangeRune.Equal(other.ChangeRune) &&
		m.ChangeAsset.Equal(other.ChangeAsset) &&
		m.LastBucket == other.LastBucket
}
