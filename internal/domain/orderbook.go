package domain

type PriceLevel struct {
	Price  float32
	Volume float32
}

type OrderBook struct {
	Exchange ExchangeEnum
	Pair     string
	Bids     []PriceLevel
	Asks     []PriceLevel
}
