package types

import (
	"fmt"
	"math/big"
	"time"
)

type Ticker struct {
	Time   time.Time
	Pair   CurrencyPair
	Price  *big.Float
	Volume *big.Float
}

func (t Ticker) String() string {
	return fmt.Sprintf(
		"s=%s p=%s, v=%s",
		t.Pair.Join("/"),
		t.Price.Text('f', -1),
		t.Volume.Text('f', -1),
	)
}
