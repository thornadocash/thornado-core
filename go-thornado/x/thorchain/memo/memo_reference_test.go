package thorchain

import (
	"testing"

	"github.com/blang/semver"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
)

type MemoReferenceSuite struct{}

var _ = Suite(&MemoReferenceSuite{})

func TestMemoReferenceSuite(t *testing.T) {
	TestingT(t)
}

func (s *MemoReferenceSuite) TestReferenceReadMemoCreateMemo(c *C) {
	// Test basic memo creation
	memo := NewReferenceReadMemo("12345")
	result := memo.CreateMemo()
	c.Assert(result, Equals, "r:12345")

	// Test with various reference lengths
	memo = NewReferenceReadMemo("00001")
	c.Assert(memo.CreateMemo(), Equals, "r:00001")

	// Test with padded references
	memo = NewReferenceReadMemo("00123")
	c.Assert(memo.CreateMemo(), Equals, "r:00123")

	// Test with full 5-digit reference
	memo = NewReferenceReadMemo("99999")
	c.Assert(memo.CreateMemo(), Equals, "r:99999")

	// Test with empty reference
	memo = NewReferenceReadMemo("")
	c.Assert(memo.CreateMemo(), Equals, "r:")

	// Test with longer reference (should still work)
	memo = NewReferenceReadMemo("123456")
	c.Assert(memo.CreateMemo(), Equals, "r:123456")
}

func (s *MemoReferenceSuite) TestReferenceReadMemoGetReference(c *C) {
	// Test GetReference method
	ref := "12345"
	memo := NewReferenceReadMemo(ref)
	c.Assert(memo.GetReference(), Equals, ref)

	// Test with empty reference
	memo = NewReferenceReadMemo("")
	c.Assert(memo.GetReference(), Equals, "")
}

func (s *MemoReferenceSuite) TestReferenceReadMemoType(c *C) {
	memo := NewReferenceReadMemo("12345")
	c.Assert(memo.GetType(), Equals, TxReferenceReadMemo)
	c.Assert(memo.IsType(TxReferenceReadMemo), Equals, true)
	c.Assert(memo.IsType(TxSwap), Equals, false)
}

func (s *MemoReferenceSuite) TestParseReferenceReadMemo(c *C) {
	// Test parsing valid reference read memo
	parsedMemo, err := ParseMemo(semver.MustParse("1.0.0"), "r:12345")
	c.Assert(err, IsNil)
	c.Assert(parsedMemo.GetType(), Equals, TxReferenceReadMemo)

	refMemo, ok := parsedMemo.(ReferenceReadMemo)
	c.Assert(ok, Equals, true)
	c.Assert(refMemo.GetReference(), Equals, "12345")

	// Test that CreateMemo produces parseable output
	original := NewReferenceReadMemo("67890")
	memoStr := original.CreateMemo()
	parsed, err := ParseMemo(semver.MustParse("1.0.0"), memoStr)
	c.Assert(err, IsNil)

	parsedRef, ok := parsed.(ReferenceReadMemo)
	c.Assert(ok, Equals, true)
	c.Assert(parsedRef.GetReference(), Equals, "67890")
}

func (s *MemoReferenceSuite) TestReferenceWriteMemo(c *C) {
	asset := common.BTCAsset
	memo := "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890"

	writeMemo := NewReferenceWriteMemo(asset, memo)
	c.Assert(writeMemo.GetAsset(), DeepEquals, asset)
	c.Assert(writeMemo.GetMemo(), Equals, memo)
	c.Assert(writeMemo.GetType(), Equals, TxReferenceWriteMemo)
}

func (s *MemoReferenceSuite) TestParseInvalidReferenceReadMemo(c *C) {
	// Test with missing reference
	_, err := ParseMemo(semver.MustParse("1.0.0"), "r:")
	c.Assert(err, NotNil)

	// Test with no parameters (should still parse but with empty reference)
	parsedMemo, err := ParseMemo(semver.MustParse("1.0.0"), "r")
	c.Assert(err, IsNil)
	refMemo, ok := parsedMemo.(ReferenceReadMemo)
	c.Assert(ok, Equals, true)
	c.Assert(refMemo.GetReference(), Equals, "")
}
