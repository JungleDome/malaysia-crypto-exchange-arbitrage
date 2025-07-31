package arbitrage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"time"
)

type ArbitrageScheduledWatcher struct {
	Exchanges map[string]domain.Exchanger
	Pairs     []string
	Interval  time.Duration
	ticker    *time.Ticker
	ctx       context.Context
	Mode      domain.ArbitrageWatcherModeEnum
}

func NewArbitrageScheduledWatcher(ctx context.Context, exchanges map[string]domain.Exchanger, pairs []string, interval time.Duration, mode domain.ArbitrageWatcherModeEnum) *ArbitrageScheduledWatcher {
	return &ArbitrageScheduledWatcher{ctx: ctx, Exchanges: exchanges, Pairs: pairs, Interval: interval, Mode: mode}
}

func (watcher *ArbitrageScheduledWatcher) Start() {
	if watcher.Mode == domain.Scheduled {
		watcher.StartScheduled()
	} else if watcher.Mode == domain.Stream {
		watcher.StartStream()
	}
}

func (watcher *ArbitrageScheduledWatcher) StartScheduled() {
	watcher.ticker = time.NewTicker(watcher.Interval)
	defer watcher.ticker.Stop()

	// Run immediately first time
	for _, pair := range watcher.Pairs {
		Logger.Info("Start watching " + pair + " every " + watcher.Interval.String() + " seconds")
		Watch(pair, watcher.Exchanges, watcher.Interval)
	}

	// Then run on ticker
	for {
		select {
		case <-watcher.ctx.Done():
			Logger.Info("Stop watching")
			return
		case <-watcher.ticker.C:
			for _, pair := range watcher.Pairs {
				Watch(pair, watcher.Exchanges, watcher.Interval)
			}
		}
	}
}

func (watcher *ArbitrageScheduledWatcher) StartStream() {
	for _, exchange := range watcher.Exchanges {
		for _, pair := range watcher.Pairs {
			Logger.Info("Start streaming " + pair + " on " + exchange.GetName())
			exchange.SubscribeSocket(watcher.ctx, pair)
		}
	}
}

// func (watcher *ArbitrageScheduledWatcher) StartWatching(ctx context.Context, pair string, interval time.Duration) {
// 	ticker := time.NewTicker(interval)
// 	defer ticker.Stop()

// 	Logger.Info("Start watching " + pair + " every " + interval.String() + " seconds")

// 	// Run immediately first time
// 	Watch(pair, watcher.Exchanges, interval)

// 	// Then run on ticker
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			Logger.Info("Stop watching " + pair)
// 			return
// 		case <-ticker.C:
// 			Watch(pair, watcher.Exchanges, interval)
// 		}
// 	}
// }

func Watch(pair string, exchanges map[string]domain.Exchanger, timeoutInterval time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutInterval)
	defer cancel()

	orderbooks, err := getOrderBookFromApi(ctx, exchanges, pair)
	if err != nil {
		Logger.Error("Failed to get order books: " + err.Error())
		return
	}

	arbitrageOutput, _ := Analyze(orderbooks[0], orderbooks[1])

	for _, arbitrageOutput := range arbitrageOutput {
		if !checkArbitrageOutput(&arbitrageOutput) {
			logArbitrageOutput(&arbitrageOutput)
			continue
		}

		// Get exchanges using BuyOn/SellOn as keys
		buyExchange := exchanges[arbitrageOutput.BuyOn]
		sellExchange := exchanges[arbitrageOutput.SellOn]

		// Get transfer fee if not already set
		if arbitrageOutput.IsDynamicTransferFee {
			transferFee, err := getTransferFeeFromApi(ctx, buyExchange, sellExchange, arbitrageOutput.Pair, arbitrageOutput.BuyVolume)
			if err != nil {
				Logger.Error("Failed to get transfer fees: " + err.Error())
				return
			}
			arbitrageOutput.NativeTransferFee = transferFee
		}

		arbitrageOutput.TransferFee = arbitrageOutput.NativeTransferFee * arbitrageOutput.BuyPrice
		arbitrageOutput.SellVolume = arbitrageOutput.BuyVolume - arbitrageOutput.NativeTransferFee
		arbitrageOutput.NetProfit = arbitrageOutput.GetNetProfit()
		arbitrageOutput.Profitable = arbitrageOutput.NetProfit > 0

		logArbitrageOutput(&arbitrageOutput)

		// Check withdrawal minimum on buy exchange
		withdrawMin, err := buyExchange.GetWithdrawMin(arbitrageOutput.Pair)
		if err != nil {
			Logger.Error("Failed to get withdrawal minimum: " + err.Error())
			return
		}
		if arbitrageOutput.BuyVolume < withdrawMin {
			Logger.Info(fmt.Sprintf("Buy volume %v is below withdrawal minimum %v", arbitrageOutput.BuyVolume, withdrawMin))
			continue
		}

		// Check deposit minimum on sell exchange
		depositMin, err := sellExchange.GetDepositMin(arbitrageOutput.Pair)
		if err != nil {
			Logger.Error("Failed to get deposit minimum: " + err.Error())
			return
		}
		if arbitrageOutput.SellVolume < depositMin {
			Logger.Info(fmt.Sprintf("Sell volume %v is below deposit minimum %v", arbitrageOutput.SellVolume, depositMin))
			continue
		}

		if arbitrageOutput.Profitable && arbitrageOutput.NetProfit >= 2 {
			AlertDiscord(arbitrageOutput)
		}
	}
}

func getOrderBookFromApi(ctx context.Context, exchanges map[string]domain.Exchanger, pair string) ([]domain.OrderBook, error) {
	orderbooks := make([]domain.OrderBook, len(exchanges))
	errCh := make(chan error, len(exchanges))
	i := 0

	for _, ex := range exchanges {
		go func(i int, ex domain.Exchanger) {
			orderbook, err := ex.GetCurrentOrderBook(pair)
			if err != nil {
				errCh <- err
				return
			}
			orderbooks[i] = orderbook
			errCh <- nil
		}(i, ex)
		i++
	}

	for i := range exchanges {
		select {
		case <-ctx.Done():
			Logger.Error("Timeout while getting order books for " + exchanges[i].GetName() + " Symbol:" + pair)
			return nil, errors.New("timeout while getting order books for " + exchanges[i].GetName() + " Symbol:" + pair)
		case err := <-errCh:
			if err != nil {
				Logger.Error("Failed to get order book for " + exchanges[i].GetName() + " Symbol:" + pair + " Error:" + err.Error())
				return nil, err
			}
		}
	}

	return orderbooks, nil
}

func getTransferFeeFromApi(ctx context.Context, fromExchange domain.Exchanger, toExchange domain.Exchanger, pair string, amount float32) (float32, error) {
	errCh := make(chan error, 1)
	var transferFee float32
	var depositAddress string
	var err error

	go func() {
		depositAddress, err = toExchange.GetDepositAddress(pair)
		if err != nil {
			errCh <- err
			return
		}

		transferFee, err = fromExchange.GetTransferFee(pair, depositAddress, amount)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		Logger.Error("Timeout while getting transfer fee for " + fromExchange.GetName() + " Symbol:" + pair)
		return 0, errors.New("timeout while getting transfer fee for " + fromExchange.GetName() + " Symbol:" + pair)
	case err := <-errCh:
		if err != nil {
			Logger.Error("Failed to get transfer fee for " + fromExchange.GetName() + " Symbol:" + pair + " Error:" + err.Error())
			return 0, err
		}
	}

	return transferFee, nil
}

func checkArbitrageOutput(arbitrageOutput *domain.ArbitrageOpportunity) bool {
	if arbitrageOutput == nil {
		return false
	}

	if arbitrageOutput.BuyPrice > arbitrageOutput.SellPrice {
		Logger.Error("Buy price is greater than sell price for " + arbitrageOutput.Pair + " on " + arbitrageOutput.BuyOn + " and " + arbitrageOutput.SellOn)
		return false
	}

	if arbitrageOutput.BuyVolume != arbitrageOutput.SellVolume {
		Logger.Error("Buy volume is not equal to sell volume for " + arbitrageOutput.Pair + " on " + arbitrageOutput.BuyOn + " and " + arbitrageOutput.SellOn)
		return false
	}

	return true
}

func logArbitrageOutput(arbitrageOutput *domain.ArbitrageOpportunity) {
	if arbitrageOutput != nil {
		jsonBytes, err := json.Marshal(arbitrageOutput)
		if err != nil {
			Logger.Error("Failed to marshal arbitrage output: " + err.Error())
		}
		ArbitrageLogger.Info(string(jsonBytes))
	}
}
