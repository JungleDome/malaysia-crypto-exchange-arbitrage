package luno

import "fmt"

type SequenceIncorrectError struct {
	ExpectedSequence int
	ActualSequence   int
}

func (e *SequenceIncorrectError) Error() string {
	return fmt.Sprintf("sequence number mismatch. Expected: %d, got: %d", e.ExpectedSequence, e.ActualSequence)
}

type LunoWebsocketAuthenticationRequest struct {
	ApiKeyId     string `json:"api_key_id"`
	ApiKeySecret string `json:"api_key_secret"`
}

type LunoOrderBookFeedSnapshot struct {
	Sequence  int                      `json:"sequence,string"`
	Asks      []LunoOrderBookPriceFeed `json:"asks"`
	Bids      []LunoOrderBookPriceFeed `json:"bids"`
	Status    string                   `json:"status"`
	Timestamp int                      `json:"timestamp"`
}

type LunoOrderBookPriceFeed struct {
	Id     string  `json:"id"`
	Price  float32 `json:"price,string"`
	Volume float32 `json:"volume,string"`
}

type LunoOrderBookFeedMessage struct {
	Sequence     int                            `json:"sequence,string"`
	TradeUpdates []LunoOrderBookFeedTradeUpdate `json:"trade_updates"`
	CreateUpdate *LunoOrderBookFeedCreateUpdate `json:"create_update"`
	DeleteUpdate *LunoOrderBookFeedDeleteUpdate `json:"delete_update"`
	StatusUpdate *LunoOrderBookFeedStatusUpdate `json:"status_update"`
	Timestamp    int                            `json:"timestamp"`
}

type LunoOrderBookFeedTradeUpdate struct {
	Sequence     int     `json:"sequence"`
	Base         float32 `json:"base,string"`
	Counter      float32 `json:"counter,string"`
	MakerOrderId string  `json:"maker_order_id"`
	TakerOrderId string  `json:"taker_order_id"`
}

type LunoOrderBookFeedCreateUpdate struct {
	OrderId string  `json:"order_id"`
	Type    string  `json:"type"`
	Price   float32 `json:"price,string"`
	Volume  float32 `json:"volume,string"`
}

type LunoOrderBookFeedDeleteUpdate struct {
	OrderId string `json:"order_id"`
}

type LunoOrderBookFeedStatusUpdate struct {
	Status string `json:"status"`
}
