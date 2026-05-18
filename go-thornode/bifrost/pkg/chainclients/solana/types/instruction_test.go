package types

import (
	"testing"

	. "gopkg.in/check.v1"
)

func InstructionTestPackage(t *testing.T) { TestingT(t) }

type InstructionTestSuite struct{}

var _ = Suite(&InstructionTestSuite{})

func (s *InstructionTestSuite) TestNewCompiledInstruction(c *C) {
	i := NewCompiledInstruction(1, []int{2}, []byte{1, 2, 3, 4})
	c.Assert(i.ProgramIDIndex, Equals, 1)
}

func (s *InstructionTestSuite) TestNewCompiledTransferInstruction(c *C) {
	i := NewCompiledTransferInstruction(1, 2, 3, 10000)
	c.Assert(i.ProgramIDIndex, Equals, 1)
	c.Assert(i.Accounts, DeepEquals, []int{2, 3})
	c.Assert(i.Data, DeepEquals, []byte{2, 0, 0, 0, 16, 39, 0, 0, 0, 0, 0, 0})
}
