package domain

type ArbitrageOpportunity struct {
	Pair           string
	BuyOn          string
	SellOn         string
	BuyPrice       float32
	BuyVolume      float32
	BuyFee         float32
	TotalBuyPrice  float32
	SellPrice      float32
	SellVolume     float32
	SellFee        float32
	TotalSellPrice float32
	PriceDiff      float32
	TransferFee    float32 // calculated using sell price using sell currency (myr)
	NetProfit      float32
	Profitable     bool
	BuyOrders      []PriceLevel
	SellOrders     []PriceLevel
}
