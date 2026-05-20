package common

import (
	"fmt"
	"math/big"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

type TypeConvertTestSuite struct{}

var _ = Suite(&TypeConvertTestSuite{})

// GetUncappedShareV1 is the old, slower implementation replaced by GetUncappedShare in production code.
// It is kept in tests only for regression checks and benchmark comparisons.
func GetUncappedShareV1(part, total, allocation cosmos.Uint) (share cosmos.Uint) {
	if part.IsZero() || total.IsZero() {
		return cosmos.ZeroUint()
	}
	defer func() {
		if err := recover(); err != nil {
			share = cosmos.ZeroUint()
		}
	}()
	// use string to convert cosmos.Uint to cosmos.Dec is the only way I can find out without being constrain to uint64
	// cosmos.Uint can hold values way larger than uint64 , because it is using big.Int internally
	aD, err := cosmos.NewDecFromStr(allocation.String())
	if err != nil {
		panic(fmt.Errorf("fail to convert %s to cosmos.Dec: %w", allocation.String(), err))
	}

	pD, err := cosmos.NewDecFromStr(part.String())
	if err != nil {
		panic(fmt.Errorf("fail to convert %s to cosmos.Dec: %w", part.String(), err))
	}
	tD, err := cosmos.NewDecFromStr(total.String())
	if err != nil {
		panic(fmt.Errorf("fail to convert%s to cosmos.Dec: %w", total.String(), err))
	}
	// A / (Total / part) == A * (part/Total) but safer when part < Totals
	result := aD.Quo(tD.Quo(pD))
	share = cosmos.NewUintFromBigInt(result.RoundInt().BigInt())
	return
}

func (TypeConvertTestSuite) TestSafeSub(c *C) {
	input1 := cosmos.NewUint(1)
	input2 := cosmos.NewUint(2)

	result1 := SafeSub(input2, input2)
	result2 := SafeSub(input1, input2)
	result3 := SafeSub(input2, input1)

	c.Check(result1.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result1.Uint64()))
	c.Check(result2.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result2.Uint64()))
	c.Check(result3.Equal(cosmos.NewUint(1)), Equals, true, Commentf("%d", result3.Uint64()))
	c.Check(result3.Equal(input2.Sub(input1)), Equals, true, Commentf("%d", result3.Uint64()))
}

func (TypeConvertTestSuite) TestSafeDivision(c *C) {
	input1 := cosmos.NewUint(1)
	input2 := cosmos.NewUint(2)
	total := input1.Add(input2)
	allocation := cosmos.NewUint(100000000)

	result1 := GetUncappedShareV1(input1, total, allocation)
	c.Check(result1.Equal(cosmos.NewUint(33333333)), Equals, true, Commentf("%d", result1.Uint64()))

	result2 := GetUncappedShareV1(input2, total, allocation)
	c.Check(result2.Equal(cosmos.NewUint(66666667)), Equals, true, Commentf("%d", result2.Uint64()))

	result3 := GetUncappedShareV1(cosmos.ZeroUint(), total, allocation)
	c.Check(result3.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result3.Uint64()))

	result4 := GetUncappedShareV1(input1, cosmos.ZeroUint(), allocation)
	c.Check(result4.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result4.Uint64()))

	result5 := GetUncappedShareV1(input1, total, cosmos.ZeroUint())
	c.Check(result5.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result5.Uint64()))

	result6 := GetUncappedShareV1(cosmos.NewUint(1014), cosmos.NewUint(3), cosmos.NewUint(1000_000*One))
	c.Check(result6.Equal(cosmos.NewUint(33799999999999997)), Equals, true, Commentf("%s", result6.String()))
}

func (TypeConvertTestSuite) TestGetUncappedShare(c *C) {
	x := cosmos.NewUint(0)
	data := []byte{
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30,
	}
	z2 := new(big.Int)
	z2.SetBytes(data)
	c.Log(z2.String())
	y := cosmos.NewUintFromBigInt(z2)
	share := GetUncappedShareV1(y, cosmos.NewUint(10000), x)
	c.Assert(share.IsZero(), Equals, true)
}

// **************** GetUncappedShare V2 ****************
func (TypeConvertTestSuite) TestSafeDivisionWithGetUncappedShareV2(c *C) {
	input1 := cosmos.NewUint(1)
	input2 := cosmos.NewUint(2)
	total := input1.Add(input2)
	allocation := cosmos.NewUint(100000000)

	result1 := GetUncappedShare(input1, total, allocation)
	c.Check(result1.Equal(cosmos.NewUint(33333333)), Equals, true, Commentf("%d", result1.Uint64()))

	result2 := GetUncappedShare(input2, total, allocation)
	c.Check(result2.Equal(cosmos.NewUint(66666667)), Equals, true, Commentf("%d", result2.Uint64()))

	result3 := GetUncappedShare(cosmos.ZeroUint(), total, allocation)
	c.Check(result3.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result3.Uint64()))

	result4 := GetUncappedShare(input1, cosmos.ZeroUint(), allocation)
	c.Check(result4.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result4.Uint64()))

	result5 := GetUncappedShare(input1, total, cosmos.ZeroUint())
	c.Check(result5.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", result5.Uint64()))

	result6 := GetUncappedShare(cosmos.NewUint(1014), cosmos.NewUint(3), cosmos.NewUint(1000_000*One))
	c.Check(result6.Equal(cosmos.NewUint(33800000000000000)), Equals, true, Commentf("%s", result6.String()))
}

func (TypeConvertTestSuite) TestGetUncappedShareV2(c *C) {
	x := cosmos.NewUint(0)
	data := []byte{
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30,
	}
	z2 := new(big.Int)
	z2.SetBytes(data)
	c.Log(z2.String())
	y := cosmos.NewUintFromBigInt(z2)
	share := GetUncappedShare(y, cosmos.NewUint(10000), x)
	c.Assert(share.IsZero(), Equals, true)

	// Test division by zero behaviour
	part := cosmos.NewUint(0)
	total := cosmos.NewUint(1000)
	allocation := cosmos.NewUint(10)
	share = GetUncappedShare(part, total, allocation)
	c.Assert(share.Equal(cosmos.ZeroUint()), Equals, true)

	part = cosmos.NewUint(1)
	total = cosmos.NewUint(0)
	share = GetUncappedShare(part, total, allocation)
	c.Assert(share.Equal(cosmos.ZeroUint()), Equals, true)
}

// Test a case where a direct multiplication with cosmos.Uint would overflow
// its 256-bit representation, while the big.Int approach handles it correctly.
func (TypeConvertTestSuite) TestGetUncappedShareV2Overflow(c *C) {
	// Define two large numbers, each around 2^130.
	// They both fit within a 256-bit cosmos.Uint.
	// 2^130 in hex is "4" followed by 32 zeros.
	largeNum1, _ := new(big.Int).SetString("400000000000000000000000000000000", 16)
	largeNum2, _ := new(big.Int).SetString("500000000000000000000000000000000", 16)

	part := cosmos.NewUintFromBigInt(largeNum1)
	allocation := cosmos.NewUintFromBigInt(largeNum2)

	// The total will be larger than the part. Let's use 2*part.
	total := part.MulUint64(2)

	// The expected result should be allocation / 2
	expectedResult := allocation.QuoUint64(2)

	// 1. Test the V2 (SAFE) version
	resultV2 := GetUncappedShare(part, total, allocation)

	// We expect the result to be CORRECT.
	c.Assert(resultV2.Equal(expectedResult), Equals, true, Commentf("V2 (safe) version failed! Expected %s, got %s", expectedResult.String(), resultV2.String()))

	// 2. Test the original slow version
	resultV1 := GetUncappedShareV1(part, total, allocation)

	// We expect the result to be CORRECT (but slow).
	c.Assert(resultV1.Equal(expectedResult), Equals, true, Commentf("V1 (slow) version failed! Expected %s, got %s", expectedResult.String(), resultV1.String()))
}

// Reproduce coworker example comparing V1 vs V2 rounding/precision on large values.
// Part: 1014000000000000, Total: 3000000000000, Alloc: 4158670756597445
// Expected:
//
//	V1: 1405630715729936283
//	V2: 1405630715729936410
//	Diff: 127
func (TypeConvertTestSuite) TestGetUncappedShareV1VsV2Example(c *C) {
	part := cosmos.NewUintFromString("1014000000000000")
	total := cosmos.NewUintFromString("3000000000000")
	alloc := cosmos.NewUintFromString("4158670756597445")

	v1 := GetUncappedShareV1(part, total, alloc)
	v2 := GetUncappedShare(part, total, alloc)

	c.Assert(v1.String(), Equals, "1405630715729936283")
	c.Assert(v2.String(), Equals, "1405630715729936410")

	// Verify the stated difference of 127 units
	diff := v2.Sub(v1)
	c.Assert(diff.String(), Equals, "127")
}

// -----------------------------
// Benchmarks (std testing.B)
// -----------------------------

var benchSink cosmos.Uint

func benchmarkGetUncappedShareV1(b *testing.B, part, total, alloc cosmos.Uint) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = GetUncappedShareV1(part, total, alloc)
	}
}

func benchmarkGetUncappedShareV2(b *testing.B, part, total, alloc cosmos.Uint) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSink = GetUncappedShare(part, total, alloc)
	}
}

// Typical-size inputs (fast path under 256 bits)
func Benchmark_GetUncappedShare_V1_Small(b *testing.B) {
	part := cosmos.NewUint(1014)
	total := cosmos.NewUint(3)
	alloc := cosmos.NewUint(1000_000 * One)
	benchmarkGetUncappedShareV1(b, part, total, alloc)
}

func Benchmark_GetUncappedShare_V2_Small(b *testing.B) {
	part := cosmos.NewUint(1014)
	total := cosmos.NewUint(3)
	alloc := cosmos.NewUint(1000_000 * One)
	benchmarkGetUncappedShareV2(b, part, total, alloc)
}

// Large inputs that are likely to exceed 256-bit product and trigger big.Int fallback in V2
func Benchmark_GetUncappedShare_V1_Large(b *testing.B) {
	part := cosmos.NewUintFromString("1014000000000000")
	total := cosmos.NewUintFromString("3000000000000")
	alloc := cosmos.NewUintFromString("4158670756597445")
	benchmarkGetUncappedShareV1(b, part, total, alloc)
}

func Benchmark_GetUncappedShare_V2_Large(b *testing.B) {
	part := cosmos.NewUintFromString("1014000000000000")
	total := cosmos.NewUintFromString("3000000000000")
	alloc := cosmos.NewUintFromString("4158670756597445")
	benchmarkGetUncappedShareV2(b, part, total, alloc)
}
