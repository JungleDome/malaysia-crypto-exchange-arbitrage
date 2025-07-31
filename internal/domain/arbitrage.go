package domain

type ArbitrageOpportunity struct {
	Pair                 string
	BuyOn                string
	SellOn               string
	BuyPrice             float32
	BuyVolume            float32
	BuyFee               float32
	TotalBuyPrice        float32
	SellPrice            float32
	SellVolume           float32
	SellFee              float32
	TotalSellPrice       float32
	PriceDiff            float32
	NativeTransferFee    float32 // in native pair value (sol/avax/xrp/etc)
	TransferFee          float32
	NetProfit            float32
	Profitable           bool
	BuyOrders            []PriceLevel
	SellOrders           []PriceLevel
	IsDynamicTransferFee bool //need to acquire transfer fee from api
}

func (arbitrageOpportunity *ArbitrageOpportunity) GetNetProfit() float32 {
	return arbitrageOpportunity.TotalSellPrice - arbitrageOpportunity.TotalBuyPrice - arbitrageOpportunity.TransferFee
}
