package methods

type GasPrice struct {
	Denom   string  `json:"denom"`
	Low     float64 `json:"low"`
	Average float64 `json:"average"`
	High    float64 `json:"high"`
}
