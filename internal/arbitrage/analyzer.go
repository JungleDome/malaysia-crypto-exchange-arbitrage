package arbitrage

import orderbook "malaysia-crypto-exchange-arbitrage/internal/orderbook"

type ArbitrageOpportunity struct {
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
	NetProfit      float32
	Profitable     bool
}

func Analyze(lunoOutput []orderbook.PriceLevel, hataOutput []orderbook.PriceLevel) (output []ArbitrageOpportunity, err error) {
	// Extract prices from exchange outputs
	lunoAsk := lunoOutput[0].Price
	lunoBid := lunoOutput[1].Price
	hataAsk := hataOutput[0].Price
	hataBid := hataOutput[1].Price

	// Convert prices to float32
	lunoAskPrice := lunoAsk
	lunoBidPrice := lunoBid
	hataAskPrice := hataAsk
	hataBidPrice := hataBid

	// Calculate fees
	lunoTakerFee := float32(0.006) // 0.6%
	hataTakerFee := float32(0.004) // 0.4%
	// solTransferFee := float32(0.005) // 0.005 SOL
	solTransferFee := float32(0) // 0.005 SOL

	// Calculate price differences
	lunoPriceDiff := lunoAskPrice - hataBidPrice //1042 - 1039
	hataPriceDiff := hataAskPrice - lunoBidPrice //1040 - 1041

	// Determine best arbitrage opportunity
	if hataPriceDiff < 0 {
		// Hata ask is lower than Luno bid - consider buying on Hata and selling on Luno
		arbitrageOpportunity := &ArbitrageOpportunity{
			BuyOn:      "Hata",
			SellOn:     "Luno",
			BuyPrice:   hataAskPrice,
			BuyVolume:  hataOutput[0].Volume,
			BuyFee:     hataAskPrice * hataOutput[0].Volume * (1 + hataTakerFee),
			SellPrice:  lunoBidPrice,
			SellVolume: lunoOutput[1].Volume,
			SellFee:    lunoBidPrice * lunoOutput[1].Volume * (1 - lunoTakerFee),
			PriceDiff:  hataAskPrice - lunoBidPrice,
		}

		arbitrageOpportunity.TotalBuyPrice = arbitrageOpportunity.BuyPrice*arbitrageOpportunity.BuyVolume - arbitrageOpportunity.BuyFee
		arbitrageOpportunity.TotalSellPrice = arbitrageOpportunity.SellPrice*arbitrageOpportunity.SellVolume - arbitrageOpportunity.SellFee
		arbitrageOpportunity.NetProfit = arbitrageOpportunity.TotalSellPrice - arbitrageOpportunity.TotalBuyPrice
		arbitrageOpportunity.Profitable = arbitrageOpportunity.NetProfit > 0

		if arbitrageOpportunity.Profitable {
			output = append(output, *arbitrageOpportunity)
		}
	}
	if lunoPriceDiff < 0 {
		// Luno ask is lower than Hata bid - consider buying on Luno and selling on Hata
		buyPrice := lunoAskPrice * (1 + lunoTakerFee)
		sellPrice := hataBidPrice * (1 - hataTakerFee)
		profit := sellPrice - buyPrice - solTransferFee

		arbitrageOpportunity := &ArbitrageOpportunity{
			BuyOn:      "Luno",
			SellOn:     "Hata",
			BuyPrice:   lunoAskPrice,
			BuyVolume:  lunoOutput[0].Volume,
			BuyFee:     lunoAskPrice * lunoOutput[0].Volume * (1 + lunoTakerFee),
			SellPrice:  hataBidPrice,
			SellVolume: hataOutput[1].Volume,
			SellFee:    hataBidPrice * hataOutput[1].Volume * (1 - hataTakerFee),
			PriceDiff:  lunoAskPrice - hataBidPrice,
			Profitable: profit > 0,
		}

		arbitrageOpportunity.TotalBuyPrice = arbitrageOpportunity.BuyPrice*arbitrageOpportunity.BuyVolume - arbitrageOpportunity.BuyFee
		arbitrageOpportunity.TotalSellPrice = arbitrageOpportunity.SellPrice*arbitrageOpportunity.SellVolume - arbitrageOpportunity.SellFee
		arbitrageOpportunity.NetProfit = arbitrageOpportunity.TotalSellPrice - arbitrageOpportunity.TotalBuyPrice
		arbitrageOpportunity.Profitable = arbitrageOpportunity.NetProfit > 0

		if arbitrageOpportunity.Profitable {
			output = append(output, *arbitrageOpportunity)
		}
	}

	return output, nil

}
