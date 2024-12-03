package domain

import (
	"context"
	"sync"
)

type Exchanger interface {
	// StartOrderStream() (err error)
	SubscribeSocket(ctx context.Context, pair string) (err error)
	GetCurrentOrderBook(pair string) (output OrderBook, err error)
	GetName() string
}

type ExchangeState struct {
	OrderBook *OrderBook
	Updates   chan *OrderBook
	Stop      chan bool
	Mutex     sync.Mutex
}
