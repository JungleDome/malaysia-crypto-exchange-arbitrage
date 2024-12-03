package arbitrage

import (
	"context"
	"encoding/json"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"time"
)

type ArbitrageScheduledWatcher struct {
	Exchanges []domain.Exchanger
	Pairs     []string
	Interval  time.Duration
	ticker    *time.Ticker
	ctx       context.Context
	Mode      domain.ArbitrageWatcherModeEnum
}

func NewArbitrageScheduledWatcher(ctx context.Context, exchanges []domain.Exchanger, pairs []string, interval time.Duration, mode domain.ArbitrageWatcherModeEnum) *ArbitrageScheduledWatcher {
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

func Watch(pair string, exchanges []domain.Exchanger, timeoutInterval time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutInterval)
	defer cancel()

	orderbooks := make([]domain.OrderBook, len(exchanges))
	errCh := make(chan error, len(exchanges))

	for i, ex := range exchanges {
		go func(i int, ex domain.Exchanger) {
			orderbook, err := ex.GetCurrentOrderBook(pair)
			if err != nil {
				errCh <- err
				return
			}
			orderbooks[i] = orderbook
			errCh <- nil
		}(i, ex)
	}

	for i := range exchanges {
		select {
		case <-ctx.Done():
			Logger.Error("Timeout while getting order books for " + exchanges[i].GetName() + " Symbol:" + pair)
			return
		case err := <-errCh:
			if err != nil {
				Logger.Error("Failed to get order book for " + exchanges[i].GetName() + " Symbol:" + pair + " Error:" + err.Error())
			}
		}
	}

	arbitrageOutput, _ := Analyze(orderbooks[0], orderbooks[1])
	jsonBytes, err := json.Marshal(arbitrageOutput)
	if err != nil {
		Logger.Error("Failed to marshal arbitrage output: " + err.Error())
	}
	ArbitrageLogger.Info(string(jsonBytes))
}
