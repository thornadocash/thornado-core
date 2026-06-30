package types

import "fmt"

var (
	ErrUnavailableBlock        = fmt.Errorf("block is not yet available")
	ErrPendingErrataDelay      = fmt.Errorf("missing tx errata delay is still pending")
	ErrFailOutputMatchCriteria = fmt.Errorf("fail to get output matching criteria")
)
