package model

import (
	"strconv"

	"github.com/gnolang/tx-indexer/serve/methods"
)

// Supply is the GraphQL view of the supply of one denomination, split by
// spendability. Amounts are strings: they are int64 values that exceed
// GraphQL's 32-bit Int.
type Supply struct {
	s *methods.Supply
}

func NewSupply(s *methods.Supply) *Supply {
	return &Supply{s: s}
}

func (s *Supply) Denom() string {
	return s.s.Denom
}

func (s *Supply) Height() int64 {
	return s.s.Height
}

func (s *Supply) Total() string {
	return strconv.FormatInt(s.s.Total, 10)
}

func (s *Supply) Locked() string {
	return strconv.FormatInt(s.s.Locked, 10)
}

func (s *Supply) Spendable() string {
	return strconv.FormatInt(s.s.Spendable, 10)
}
