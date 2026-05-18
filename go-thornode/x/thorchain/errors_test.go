package thorchain

import (
	"errors"
	"fmt"
	"strings"

	errorsmod "cosmossdk.io/errors"
	"github.com/hashicorp/go-multierror"
	. "gopkg.in/check.v1"
)

type ErrorsTestSuite struct{}

var _ = Suite(&ErrorsTestSuite{})

func (ErrorsTestSuite) TestErrInternal(c *C) {
	codeErr := errBadVersion
	_, code, log := errorsmod.ABCIInfo(codeErr, false)
	c.Check(code, Equals, uint32(101))
	c.Check(strings.Contains(log, "bad version"), Equals, true)

	codelessErr := fmt.Errorf("codeless error")
	_, code, log = errorsmod.ABCIInfo(codelessErr, false)
	c.Check(int(code), Equals, 1)
	c.Check(strings.Contains(log, "codeless error"), Equals, true)

	internalErr := ErrInternal(codeErr, codelessErr.Error())
	_, code, log = errorsmod.ABCIInfo(internalErr, false)
	c.Check(int(code), Equals, 1)
	c.Check(strings.Contains(log, "codeless error"), Equals, true)

	appendedError := multierror.Append(codeErr, codelessErr)
	_, code, log = errorsmod.ABCIInfo(appendedError, false)
	c.Check(int(code), Equals, 1)
	c.Check(strings.Contains(log, "codeless error"), Equals, true)

	joinedError := errors.Join(codeErr, codelessErr)
	_, code, log = errorsmod.ABCIInfo(joinedError, false)
	c.Check(int(code), Equals, 1)
	c.Check(strings.Contains(log, "codeless error"), Equals, true)

	wrappedErr := errorsmod.Wrap(codeErr, codelessErr.Error())
	_, code, log = errorsmod.ABCIInfo(wrappedErr, false)
	c.Assert(int(code), Equals, 101) // codeErr's code preserved.
	c.Check(strings.Contains(log, "bad version"), Equals, true)
	c.Check(strings.Contains(log, "codeless error"), Equals, true)
	// Both errors' contents are preserved.
}
