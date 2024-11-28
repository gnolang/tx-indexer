package methods

type GasPrice struct {
	Denom   string `json:"denom"`
	Low     int64  `json:"low"`
	Average int64  `json:"average"`
	High    int64  `json:"high"`
}
