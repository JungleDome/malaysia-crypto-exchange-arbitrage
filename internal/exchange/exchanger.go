package exchange

import (
	"context"
	orderbook "malaysia-crypto-exchange-arbitrage/internal/orderbook"
	"sync"
)

type Exchanger interface {
	GetHighestBidPrice() (price float32, size float32, err error)
	StartOrderStream() (err error)
	SubscribeSocket(ctx context.Context, pair string) (err error)
}

type ExchangeState struct {
	OrderBook *orderbook.OrderBook
	Updates   chan *orderbook.OrderBook
	Stop      chan bool
	Mutex     sync.Mutex
}
