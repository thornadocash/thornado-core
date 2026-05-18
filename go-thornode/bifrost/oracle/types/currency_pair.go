package types

import (
	"fmt"
	"strings"
)

type CurrencyPair struct {
	Base  string
	Quote string
}

func NewCurrencyPair(str string) (CurrencyPair, error) {
	parts := strings.Split(str, "/")
	if len(parts) != 2 {
		return CurrencyPair{}, fmt.Errorf("invalid currency pair: %s", str)
	}
	return CurrencyPair{
		Base:  parts[0],
		Quote: parts[1],
	}, nil
}

func (cp CurrencyPair) String() string {
	return fmt.Sprintf("%s/%s", cp.Base, cp.Quote)
}

func (cp CurrencyPair) Join(s string) string {
	return fmt.Sprintf("%s%s%s", cp.Base, s, cp.Quote)
}

func (cp CurrencyPair) Swap() CurrencyPair {
	return CurrencyPair{cp.Quote, cp.Base}
}
