package thorchain

import (
	"fmt"
	"strings"

	"github.com/thornadocash/go-thornado/common"
)

type ReferenceWriteMemo struct {
	MemoBase
	Asset common.Asset
	Memo  string
}

func (m ReferenceWriteMemo) GetAsset() common.Asset { return m.Asset }
func (m ReferenceWriteMemo) GetMemo() string        { return m.Memo }

func NewReferenceWriteMemo(asset common.Asset, memo string) ReferenceWriteMemo {
	return ReferenceWriteMemo{
		MemoBase: MemoBase{TxType: TxReferenceWriteMemo},
		Asset:    asset,
		Memo:     memo,
	}
}

func (p *parser) ParseReferenceWriteMemo() (ReferenceWriteMemo, error) {
	return p.parseReferenceWriteMemo()
}

func (p *parser) parseReferenceWriteMemo() (ReferenceWriteMemo, error) {
	asset := p.getAsset(1, true, common.EmptyAsset)
	var memo string
	if len(p.parts) > 2 {
		parts := strings.SplitN(p.memo, ":", 3)
		if len(parts) > 2 {
			memo = parts[2]
		}
	}
	if len(memo) == 0 {
		return ReferenceWriteMemo{}, fmt.Errorf("memo cannot be blank")
	}

	// Validate that the embedded memo can be parsed correctly
	_, err := ParseMemoWithTHORNames(p.ctx, p.keeper, memo)
	if err != nil {
		return ReferenceWriteMemo{}, fmt.Errorf("embedded memo is invalid: %w", err)
	}

	return NewReferenceWriteMemo(asset, memo), p.Error()
}

type ReferenceReadMemo struct {
	MemoBase
	Reference string
}

func (m ReferenceReadMemo) GetReference() string { return m.Reference }

// CreateMemo creates a formatted reference memo string (e.g., "r:12345")
func (m ReferenceReadMemo) CreateMemo() string {
	return fmt.Sprintf("%s:%s", TxReferenceReadMemo.String(), m.Reference)
}

func NewReferenceReadMemo(ref string) ReferenceReadMemo {
	return ReferenceReadMemo{
		MemoBase:  MemoBase{TxType: TxReferenceReadMemo},
		Reference: ref,
	}
}

func (p *parser) ParseReferenceReadMemo() (ReferenceReadMemo, error) {
	return p.parseReferenceReadMemo()
}

func (p *parser) parseReferenceReadMemo() (ReferenceReadMemo, error) {
	// Check if there are enough parts to have a reference parameter
	if len(p.parts) > 1 {
		// If there's a colon but empty reference, this is an error
		if p.parts[1] == "" {
			p.addErr(fmt.Errorf("reference parameter is required when ':' is present"))
		}
		ref := p.getString(1, true, "")
		return NewReferenceReadMemo(ref), p.Error()
	}
	// No colon present, so no parameters - this is valid with empty reference
	return NewReferenceReadMemo(""), p.Error()
}
