package methods

// Supply is the supply of a single denomination at a chain height, split by
// spendability. Total is what the chain's supply counter records, Locked the
// portion held by vesting accounts under a schedule that has not vested yet,
// and Spendable the difference — the figure data aggregators call the
// circulating supply.
//
// The amounts marshal as JSON strings: they are int64 values that can exceed
// what a JavaScript number represents exactly, and aggregators consume this.
type Supply struct {
	Denom     string `json:"denom"`
	Height    int64  `json:"height"`
	Total     int64  `json:"total,string"`
	Spendable int64  `json:"spendable,string"`
	Locked    int64  `json:"locked,string"`
}
