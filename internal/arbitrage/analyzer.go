package arbitrage

import (
	"fmt"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"malaysia-crypto-exchange-arbitrage/internal/platform/config"
	"malaysia-crypto-exchange-arbitrage/internal/platform/logger"
)

var Config = config.GetConfig()
var Logger = logger.Get()
var ArbitrageLogger = logger.GetArbitrageLogger()

func Analyze(lunoOutput domain.OrderBook, hataOutput domain.OrderBook) (output []domain.ArbitrageOpportunity, err error) {
	if lunoOutput.Pair != hataOutput.Pair {
		return nil, fmt.Errorf("pair mismatch: %s != %s", lunoOutput.Pair, hataOutput.Pair)
	}
	return analyze(lunoOutput, hataOutput)
}

func analyze(firstExchangePrice domain.OrderBook, secondExchangePrice domain.OrderBook) (output []domain.ArbitrageOpportunity, err error) {
	// Extract prices from exchange outputs
	firstExchangeAskPrice := firstExchangePrice.Asks[0].Price
	firstExchangeBidPrice := firstExchangePrice.Bids[0].Price
	secondExchangeAskPrice := secondExchangePrice.Asks[0].Price
	secondExchangeBidPrice := secondExchangePrice.Bids[0].Price

	// Calculate fees
	firstExchangeTakerFee := Config.Exchange[firstExchangePrice.Exchange.String()].TakerFee
	secondExchangeTakerFee := Config.Exchange[secondExchangePrice.Exchange.String()].TakerFee
	pairTransferFee := Config.Exchange[firstExchangePrice.Exchange.String()].Crypto[firstExchangePrice.Pair].WithdrawFee //this transfer fee is used for estimating the amount we need to sell, if the transfer fee cannot be determined before the calculation we will deduct it in the next step
	slippage := Config.Arbitrage[firstExchangePrice.Pair].Slippage
	slippageMode := Config.Arbitrage[firstExchangePrice.Pair].SlippageMode

	var realPairTransferFee float32 = 0
	if pairTransferFee != -1 {
		realPairTransferFee = pairTransferFee
	}

	// Calculate price differences
	firstExchangePriceDiff := firstExchangeAskPrice - secondExchangeBidPrice  //1042 - 1039
	secondExchangePriceDiff := secondExchangeAskPrice - firstExchangeBidPrice //1040 - 1041

	// Determine best arbitrage opportunity
	if secondExchangePriceDiff < 0 {
		// Second exchange ask is lower than first exchange bid - consider buying on second exchange and selling on first exchange
		buyOrders, sellOrders, err := generatePotentialLimitOrder(secondExchangePrice, firstExchangePrice, realPairTransferFee, slippageMode, slippage)
		if err != nil {
			return nil, err
		}
		Logger.Info(secondExchangePrice.Pair + " BuyOrders: " + fmt.Sprintf("%v", buyOrders))
		Logger.Info(secondExchangePrice.Pair + " SellOrders: " + fmt.Sprintf("%v", sellOrders))

		// Calculate weighted average prices and totals from orders
		var totalBuyVolume, totalBuyAmount float32
		for _, order := range buyOrders {
			totalBuyVolume += order.Volume
			totalBuyAmount += order.Price * order.Volume
		}
		buyPrice := totalBuyAmount / totalBuyVolume       // Average price per unit
		buyFee := totalBuyAmount * secondExchangeTakerFee // Platform fee
		totalBuyPrice := totalBuyAmount + buyFee          // Total cost including fees

		var totalSellVolume, totalSellAmount float32
		for _, order := range sellOrders {
			totalSellVolume += order.Volume
			totalSellAmount += order.Price * order.Volume
		}
		sellPrice := totalSellAmount / totalSellVolume     // Average price per unit
		sellFee := totalSellAmount * firstExchangeTakerFee // Platform fee
		totalSellPrice := totalSellAmount - sellFee        // Total revenue after fees

		priceDiff := sellPrice - buyPrice // Difference in price per unit

		arbitrageOpportunity := &domain.ArbitrageOpportunity{
			Pair:                 firstExchangePrice.Pair,
			BuyOn:                secondExchangePrice.Exchange.String(),
			SellOn:               firstExchangePrice.Exchange.String(),
			BuyPrice:             buyPrice,
			BuyVolume:            totalBuyVolume,
			BuyFee:               buyFee,
			SellPrice:            sellPrice,
			SellVolume:           totalSellVolume,
			SellFee:              sellFee,
			PriceDiff:            priceDiff,
			TotalBuyPrice:        totalBuyPrice,
			TotalSellPrice:       totalSellPrice,
			NativeTransferFee:    realPairTransferFee,
			TransferFee:          realPairTransferFee * buyPrice,
			NetProfit:            totalSellPrice - totalBuyPrice - realPairTransferFee*buyPrice,
			BuyOrders:            buyOrders,
			SellOrders:           sellOrders,
			IsDynamicTransferFee: pairTransferFee == -1,
		}

		arbitrageOpportunity.Profitable = arbitrageOpportunity.NetProfit > 0

		// if arbitrageOpportunity.Profitable {
		output = append(output, *arbitrageOpportunity)
		// }
	}
	if firstExchangePriceDiff < 0 {
		// First exchange ask is lower than second exchange bid - consider buying on first exchange and selling on second exchange
		buyOrders, sellOrders, err := generatePotentialLimitOrder(firstExchangePrice, secondExchangePrice, realPairTransferFee, slippageMode, slippage)
		if err != nil {
			return nil, err
		}
		Logger.Info(firstExchangePrice.Pair + " BuyOrders: " + fmt.Sprintf("%v", buyOrders))
		Logger.Info(firstExchangePrice.Pair + " SellOrders: " + fmt.Sprintf("%v", sellOrders))

		// Calculate weighted average prices and totals from orders
		var totalBuyVolume, totalBuyAmount float32
		for _, order := range buyOrders {
			totalBuyVolume += order.Volume
			totalBuyAmount += order.Price * order.Volume
		}
		buyPrice := totalBuyAmount / totalBuyVolume      // Average price per unit
		buyFee := totalBuyAmount * firstExchangeTakerFee // Platform fee
		totalBuyPrice := totalBuyAmount + buyFee         // Total cost including fees

		var totalSellVolume, totalSellAmount float32
		for _, order := range sellOrders {
			totalSellVolume += order.Volume
			totalSellAmount += order.Price * order.Volume
		}
		sellPrice := totalSellAmount / totalSellVolume      // Average price per unit
		sellFee := totalSellAmount * secondExchangeTakerFee // Platform fee
		totalSellPrice := totalSellAmount - sellFee         // Total revenue after fees

		priceDiff := sellPrice - buyPrice // Difference in price per unit

		arbitrageOpportunity := &domain.ArbitrageOpportunity{
			Pair:                 firstExchangePrice.Pair,
			BuyOn:                firstExchangePrice.Exchange.String(),
			SellOn:               secondExchangePrice.Exchange.String(),
			BuyPrice:             buyPrice,
			BuyVolume:            totalBuyVolume,
			BuyFee:               buyFee,
			SellPrice:            sellPrice,
			SellVolume:           totalSellVolume,
			SellFee:              sellFee,
			PriceDiff:            priceDiff,
			TotalBuyPrice:        totalBuyPrice,
			TotalSellPrice:       totalSellPrice,
			NativeTransferFee:    realPairTransferFee,
			TransferFee:          realPairTransferFee * buyPrice,
			NetProfit:            totalSellPrice - totalBuyPrice - realPairTransferFee*buyPrice,
			BuyOrders:            buyOrders,
			SellOrders:           sellOrders,
			IsDynamicTransferFee: pairTransferFee == -1,
		}

		arbitrageOpportunity.Profitable = arbitrageOpportunity.NetProfit > 0

		// if arbitrageOpportunity.Profitable {
		output = append(output, *arbitrageOpportunity)
		// }
	}

	return output, nil

}

func generatePotentialLimitOrder(buyOrderbook domain.OrderBook, sellOrderbook domain.OrderBook, transferFee float32, slippageMode domain.SlippageDetectionModeEnum, slippage float32) (buyOrder []domain.PriceLevel, sellOrder []domain.PriceLevel, err error) {
	const maxMYR float32 = 5000

	// Step 1: Find asks within slippage
	lowestAskPrice := buyOrderbook.Asks[0].Price
	var maxAskPrice float32
	if slippageMode == domain.Price {
		maxAskPrice = lowestAskPrice + slippage
	} else {
		maxAskPrice = lowestAskPrice * (1 + slippage)
	}

	var eligibleAsks []domain.PriceLevel
	var totalBuyAmount float32

	for _, ask := range buyOrderbook.Asks {
		if ask.Price > maxAskPrice {
			break
		}

		orderAmount := ask.Price * ask.Volume
		if totalBuyAmount+orderAmount > maxMYR {
			// Calculate partial volume that fits within maxMYR
			remainingMYR := maxMYR - totalBuyAmount
			partialVolume := remainingMYR / ask.Price
			eligibleAsks = append(eligibleAsks, domain.PriceLevel{
				Price:  ask.Price,
				Volume: partialVolume,
			})
			break
		}

		eligibleAsks = append(eligibleAsks, ask)
		totalBuyAmount += orderAmount
	}

	if len(eligibleAsks) == 0 {
		return buyOrder, sellOrder, fmt.Errorf("no eligible asks found within slippage")
	}

	// Step 2: Find matching bids within slippage
	highestBidPrice := sellOrderbook.Bids[0].Price
	var minBidPrice float32
	if slippageMode == domain.Price {
		minBidPrice = highestBidPrice - slippage
	} else {
		minBidPrice = highestBidPrice * (1 - slippage)
	}

	var eligibleBids []domain.PriceLevel
	for _, bid := range sellOrderbook.Bids {
		if bid.Price < minBidPrice {
			break
		}
		eligibleBids = append(eligibleBids, bid)
	}

	if len(eligibleBids) == 0 {
		return buyOrder, sellOrder, fmt.Errorf("no eligible bids found within slippage")
	}

	// Step 3: Calculate total crypto amount after buying & transfer fee
	var totalCryptoAmount float32
	for _, ask := range eligibleAsks {
		totalCryptoAmount += ask.Volume
	}
	totalCryptoAmount = totalCryptoAmount - transferFee

	// Step 4: Match with bid orders
	var totalBidVolume float32
	var finalBidVolume float32

	for _, bid := range eligibleBids {
		if totalBidVolume+bid.Volume >= totalCryptoAmount {
			// Case 4.1: Current bid order completes the total crypto amount
			// remainingVolume := totalCryptoAmount - totalBidVolume
			finalBidVolume = totalCryptoAmount
			break
		}
		// Case 4.2: Need more bid orders
		totalBidVolume += bid.Volume
		finalBidVolume = totalBidVolume
	}

	if finalBidVolume < totalCryptoAmount {
		// Adjust buy amount to match available sell volume
		totalCryptoAmount = finalBidVolume
	}

	// Create buy orders from eligible asks
	buyOrder = make([]domain.PriceLevel, 0)
	var accumulatedBuyVolume float32
	for _, ask := range eligibleAsks {
		if accumulatedBuyVolume >= totalCryptoAmount {
			break
		}

		var volume float32
		if accumulatedBuyVolume+ask.Volume > totalCryptoAmount {
			volume = totalCryptoAmount - accumulatedBuyVolume
		} else {
			volume = ask.Volume
		}

		buyOrder = append(buyOrder, domain.PriceLevel{
			Price:  ask.Price,
			Volume: volume,
		})
		accumulatedBuyVolume += volume
	}

	// Create sell orders from eligible bids
	sellOrder = make([]domain.PriceLevel, 0)
	var accumulatedSellVolume float32
	for _, bid := range eligibleBids {
		if accumulatedSellVolume >= totalCryptoAmount {
			break
		}

		var volume float32
		if accumulatedSellVolume+bid.Volume > totalCryptoAmount {
			volume = totalCryptoAmount - accumulatedSellVolume
		} else {
			volume = bid.Volume
		}

		sellOrder = append(sellOrder, domain.PriceLevel{
			Price:  bid.Price,
			Volume: volume,
		})
		accumulatedSellVolume += volume
	}

	return buyOrder, sellOrder, nil
	//Step 2: Get the matched bid order in the sell orderbook considering the slippage. (This is to get the total volume we can sell)
	//Step 3: Get the total crypto amount after buying & deducting the transfer fee.
	//Step 4: From the matched bid order
	//Step 4.1: If total volume of bid order is more than the total crypto amount, then we just generate the limit order
	//Step 4.2: If total volume of bid order is less than the total crypto amount, then we need to adjust the total crypto amount we can buy and find the volume of ask order that matches the total crypto amount, and generate the limit order
	//Step 5: Find the average price * volume of the asks.

}
