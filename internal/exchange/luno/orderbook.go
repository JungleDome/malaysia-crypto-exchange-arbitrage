package luno

import "fmt"

func (lunoExchange *LunoExchange) GetLowestAskPrice(pair string) (price float32, size float32, err error) {
	if state, ok := lunoExchange.states[pair]; ok {
		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.OrderBook != nil && len(state.OrderBook.Asks) > 0 {
			// Return lowest ask price (first ask in sorted order)
			lowestAsk := state.OrderBook.Asks[0]
			return lowestAsk.Price, lowestAsk.Volume, nil
		}
		return 0, 0, fmt.Errorf("no asks available in order book for pair %s", pair)
	}
	return 0, 0, fmt.Errorf("no state found for pair %s", pair)
}

func (lunoExchange *LunoExchange) GetLowestAskPriceByVolume(pair string, volume float32, exactMatch bool) (price float32, totalVolume float32, err error) {
	if state, ok := lunoExchange.states[pair]; ok {
		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.OrderBook != nil && len(state.OrderBook.Asks) > 0 {
			// Accumulate volume until we reach target
			var accumulatedVolume float32
			var weightedPrice float32

			for _, ask := range state.OrderBook.Asks {
				remaining := volume - accumulatedVolume
				if remaining <= 0 {
					break
				}

				volumeToAdd := ask.Volume
				if volumeToAdd > remaining {
					volumeToAdd = remaining
				}

				weightedPrice += ask.Price * volumeToAdd
				accumulatedVolume += volumeToAdd
			}

			if exactMatch && accumulatedVolume < volume {
				return 0, 0, fmt.Errorf("insufficient volume in order book for pair %s", pair)
			}

			if accumulatedVolume > 0 {
				return weightedPrice / accumulatedVolume, accumulatedVolume, nil
			}
			return 0, 0, fmt.Errorf("no volume available in order book for pair %s", pair)
		}
		return 0, 0, fmt.Errorf("no asks available in order book for pair %s", pair)
	}
	return 0, 0, fmt.Errorf("no state found for pair %s", pair)
}

func (lunoExchange *LunoExchange) GetHighestBidPrice(pair string) (price float32, size float32, err error) {
	if state, ok := lunoExchange.states[pair]; ok {
		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.OrderBook != nil && len(state.OrderBook.Bids) > 0 {
			// Return highest bid price (first bid in sorted order)
			highestBid := state.OrderBook.Bids[0]
			return highestBid.Price, highestBid.Volume, nil
		}
		return 0, 0, fmt.Errorf("no bids available in order book for pair %s", pair)
	}
	return 0, 0, fmt.Errorf("no state found for pair %s", pair)
}

func (lunoExchange *LunoExchange) GetHighestBidPriceByVolume(pair string, volume float32, exactMatch bool) (price float32, totalVolume float32, err error) {
	if state, ok := lunoExchange.states[pair]; ok {
		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.OrderBook != nil && len(state.OrderBook.Bids) > 0 {
			// Accumulate volume until we reach target
			var accumulatedVolume float32
			var weightedPrice float32

			for _, bid := range state.OrderBook.Bids {
				remaining := volume - accumulatedVolume
				if remaining <= 0 {
					break
				}

				volumeToAdd := bid.Volume
				if volumeToAdd > remaining {
					volumeToAdd = remaining
				}

				weightedPrice += bid.Price * volumeToAdd
				accumulatedVolume += volumeToAdd
			}

			if exactMatch && accumulatedVolume < volume {
				return 0, 0, fmt.Errorf("insufficient volume in order book for pair %s", pair)
			}

			if accumulatedVolume > 0 {
				return weightedPrice / accumulatedVolume, accumulatedVolume, nil
			}
			return 0, 0, fmt.Errorf("no volume available in order book for pair %s", pair)
		}
		return 0, 0, fmt.Errorf("no bids available in order book for pair %s", pair)
	}
	return 0, 0, fmt.Errorf("no state found for pair %s", pair)
}
