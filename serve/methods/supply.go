package methods

// Supply is the supply of a single denomination at a chain height, split
// by spendability. Total is what the chain's supply counter records,
// Locked is the portion held by vesting accounts whose schedule has not
// fully vested, and Spendable is the difference. Data aggregators call
// that last number the circulating supply.
//
// Amounts marshal as JSON strings: they are int64 values that can exceed
// what a JavaScript number represents exactly, and aggregators read this
// endpoint.
type Supply struct {
	Denom     string `json:"denom"`
	Height    int64  `json:"height"`
	Total     int64  `json:"total,string"`
	Spendable int64  `json:"spendable,string"`
	Locked    int64  `json:"locked,string"`
}
