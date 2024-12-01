package orderbook

type PriceLevel struct {
	Price  float32
	Volume float32
}

type OrderBook struct {
	Pair string
	Bids []PriceLevel
	Asks []PriceLevel
}
