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
	GetTransferFee(pair string, address string, amount float32) (fee float32, err error)
	GetWithdrawMin(pair string) (min float32, err error)
	GetDepositMin(pair string) (min float32, err error)
	GetDepositAddress(pair string) (address string, err error)
}

type ExchangeState struct {
	OrderBook *OrderBook
	Updates   chan *OrderBook
	Stop      chan bool
	Mutex     sync.Mutex
}
