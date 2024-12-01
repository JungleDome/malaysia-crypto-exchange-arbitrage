package luno

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"malaysia-crypto-exchange-arbitrage/internal/config"
	exchange "malaysia-crypto-exchange-arbitrage/internal/exchange"
	orderbook "malaysia-crypto-exchange-arbitrage/internal/orderbook"
	logger "malaysia-crypto-exchange-arbitrage/internal/utils"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/luno/luno-go"
)

type LunoExchange struct {
	lunoClient       luno.Client
	websocketBaseUrl string
	apiKeyId         string
	apiKeySecret     string
	states           map[string]*LunoExchangeState
}

type LunoExchangeState struct {
	exchange.ExchangeState
	AsksOrderbook   []LunoOrderBookPriceFeed
	BidsOrderbook   []LunoOrderBookPriceFeed
	CurrentSequence int
}

const lunoWebsocketBaseUrl = "wss://ws.luno.com/api/1/stream/"

var Logger = logger.Get()
var StateLogger = logger.GetStateLogger()

func CreateClient(id string, secret string) *LunoExchange {
	lunoClient := luno.NewClient()
	lunoClient.SetAuth(id, secret)

	Logger.Info("Luno client created")

	return &LunoExchange{
		lunoClient:       *lunoClient,
		websocketBaseUrl: lunoWebsocketBaseUrl,
		apiKeyId:         id,
		apiKeySecret:     secret,
		states:           make(map[string]*LunoExchangeState),
	}

}

func (lunoExchange *LunoExchange) TestClient() (output []orderbook.PriceLevel, err error) {
	req := luno.GetOrderBookRequest{Pair: "SOLMYR"}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()

	res, err := lunoExchange.lunoClient.GetOrderBook(ctx, &req)
	if err != nil {
		log.Printf("Failed to get Luno order book: %v", err)
	}
	// log.Println(res)

	log.Println(res.Asks[len(res.Asks)-1]) //<-- highest ask price
	log.Println(res.Asks[0])               //<-- lowest ask price

	log.Println(res.Bids[len(res.Bids)-1]) //<-- lowest bid price
	log.Println(res.Bids[0])               //<-- highest bid price

	output = append(output, orderbook.PriceLevel{
		Price:  float32(res.Asks[0].Price.Float64()),
		Volume: float32(res.Asks[0].Volume.Float64()),
	})
	output = append(output, orderbook.PriceLevel{
		Price:  float32(res.Bids[0].Price.Float64()),
		Volume: float32(res.Bids[0].Volume.Float64()),
	})

	return output, nil
}

func (lunoExchange *LunoExchange) SubscribeSocket(ctx context.Context, pair string) (err error) {
	Logger.Info("Subscribing to Luno websocket for pair: " + pair)

	c, _, err := websocket.Dial(ctx, lunoExchange.websocketBaseUrl+pair, nil)
	c.SetReadLimit(-1) //Disable read limit
	if err != nil {
		Logger.Error("Failed to dial Luno websocket: " + err.Error())
		return err
	}

	err = lunoExchange.sendAuthenticationMessage(ctx, c)
	if err != nil {
		return err
	}

	// Start goroutine to read messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				Logger.Info("Received interrupt signal. Closing Luno websocket connection.")
				c.Close(websocket.StatusNormalClosure, "")
				return
			default:
				messageType, message, err := c.Read(ctx)
				if err != nil {
					Logger.Error("Failed to read message from Luno websocket: " + err.Error())
					return
				}
				if messageType == websocket.MessageText {
					Logger.Info("Received message from Luno websocket. Message: " + string(message))
					err := lunoExchange.processOrderBookFeed(message, pair)
					if err != nil {
						Logger.Error("Failed to process Luno order book feed: " + err.Error())
					}
				} else {
					Logger.Error("Received unknown message type from Luno websocket: " + strconv.Itoa(int(messageType)))
				}
			}
		}
	}()

	return
}

func (lunoExchange *LunoExchange) sendAuthenticationMessage(ctx context.Context, c *websocket.Conn) error {
	//Send authentication message
	authMessage := LunoWebsocketAuthenticationRequest{
		ApiKeyId:     lunoExchange.apiKeyId,
		ApiKeySecret: lunoExchange.apiKeySecret,
	}
	authMessageBytes, err := json.Marshal(authMessage)
	if err != nil {
		Logger.Error("Failed to marshal authentication message: " + err.Error())
		return err
	}

	Logger.Info("Sending authentication message to Luno websocket.")
	err = c.Write(ctx, websocket.MessageText, authMessageBytes)
	if err != nil {
		Logger.Error("Failed to send authentication message to Luno websocket: " + err.Error())
		return err
	}

	return nil
}

func (lunoExchange *LunoExchange) processOrderBookFeed(feedString []byte, pair string) error {
	config := config.GetConfig()
	maxPriceDiff := config.MarketFilter[pair].MaxPriceDiff

	if lunoExchange.states[pair] == nil || lunoExchange.states[pair].CurrentSequence == 0 {
		var feedSnapshot *LunoOrderBookFeedSnapshot
		err := json.Unmarshal(feedString, &feedSnapshot)
		if err != nil {
			return fmt.Errorf("failed to unmarshal Luno order book feed snapshot: %v", err)
		}

		lunoExchange.states[pair] = &LunoExchangeState{
			ExchangeState: exchange.ExchangeState{
				OrderBook: &orderbook.OrderBook{Pair: pair},
				Updates:   make(chan *orderbook.OrderBook),
				Stop:      make(chan bool),
				Mutex:     sync.Mutex{},
			},
			AsksOrderbook:   make([]LunoOrderBookPriceFeed, 0),
			BidsOrderbook:   make([]LunoOrderBookPriceFeed, 0),
			CurrentSequence: feedSnapshot.Sequence,
		}

		if feedSnapshot != nil {
			err := lunoExchange.processFeedSnapshot(feedSnapshot, pair, maxPriceDiff)
			StateLogger.Info("Current internal state for pair: " + pair + " is: " + fmt.Sprintf("%v", lunoExchange.states[pair]))
			if err != nil {
				return err
			}
		}
	}

	var feedMessage *LunoOrderBookFeedMessage
	err2 := json.Unmarshal(feedString, &feedMessage)
	if err2 != nil {
		return fmt.Errorf("failed to unmarshal Luno order book feed: %v", err2)
	}

	if feedMessage != nil {
		lunoExchange.states[pair].Mutex.Lock()
		defer lunoExchange.states[pair].Mutex.Unlock()

		err := lunoExchange.processFeedUpdate(feedMessage, pair, maxPriceDiff)
		StateLogger.Info("Current internal state for pair: " + pair + " is: " + fmt.Sprintf("%v", lunoExchange.states[pair]))
		if err != nil {
			return err
		}
	}

	return nil
}

func (lunoExchange *LunoExchange) processFeedSnapshot(feedSnapshot *LunoOrderBookFeedSnapshot, pair string, maxPriceDiff float32) error {
	// Process asks
	asks := make([]orderbook.PriceLevel, 0)
	StateLogger.Info("Feed snapshot for pair: " + pair + " is: " + fmt.Sprintf("%v", feedSnapshot))
	lowestAskPrice := feedSnapshot.Asks[0].Price // First ask is lowest
	for _, ask := range feedSnapshot.Asks {
		// Only include asks within maxPriceDiff of lowest ask
		priceDiff := (ask.Price - lowestAskPrice) / lowestAskPrice
		if priceDiff <= maxPriceDiff {
			asks = append(asks, orderbook.PriceLevel{
				Price:  ask.Price,
				Volume: ask.Volume,
			})
			lunoExchange.states[pair].AsksOrderbook = append(lunoExchange.states[pair].AsksOrderbook, ask)
		}
	}

	// Process bids
	bids := make([]orderbook.PriceLevel, 0)
	highestBidPrice := feedSnapshot.Bids[0].Price // First bid is highest
	for _, bid := range feedSnapshot.Bids {
		// Only include bids within maxPriceDiff of highest bid
		priceDiff := (highestBidPrice - bid.Price) / highestBidPrice
		if priceDiff <= maxPriceDiff {
			bids = append(bids, orderbook.PriceLevel{
				Price:  bid.Price,
				Volume: bid.Volume,
			})
			lunoExchange.states[pair].BidsOrderbook = append(lunoExchange.states[pair].BidsOrderbook, bid)
		}
	}

	lunoExchange.states[pair].OrderBook = &orderbook.OrderBook{
		Pair: pair,
		Asks: asks,
		Bids: bids,
	}

	// Send update to subscribers
	select {
	case lunoExchange.states[pair].Updates <- lunoExchange.states[pair].OrderBook:
	default:
	}
	return nil
}

func (lunoExchange *LunoExchange) processFeedUpdate(feedMessage *LunoOrderBookFeedMessage, pair string, maxPriceDiff float32) error {
	// Process trade events
	if feedMessage.TradeUpdates != nil {
		for _, trade := range feedMessage.TradeUpdates {
			// Find and update the maker order
			found := false

			// Check asks first
			for i, ask := range lunoExchange.states[pair].AsksOrderbook {
				if ask.Id == trade.MakerOrderId {
					newVolume := ask.Volume - trade.Base
					if newVolume <= 0 {
						// Remove from both AsksOrderbook and OrderBook.Asks
						lunoExchange.states[pair].AsksOrderbook = append(
							lunoExchange.states[pair].AsksOrderbook[:i],
							lunoExchange.states[pair].AsksOrderbook[i+1:]...)
						lunoExchange.states[pair].OrderBook.Asks = append(
							lunoExchange.states[pair].OrderBook.Asks[:i],
							lunoExchange.states[pair].OrderBook.Asks[i+1:]...)
					} else {
						// Update volume in both places
						lunoExchange.states[pair].AsksOrderbook[i].Volume = newVolume
						lunoExchange.states[pair].OrderBook.Asks[i].Volume = newVolume
					}
					found = true
					break
				}
			}

			// If not found in asks, check bids
			if !found {
				for i, bid := range lunoExchange.states[pair].BidsOrderbook {
					if bid.Id == trade.MakerOrderId {
						newVolume := bid.Volume - trade.Base
						if newVolume <= 0 {
							lunoExchange.states[pair].BidsOrderbook = append(
								lunoExchange.states[pair].BidsOrderbook[:i],
								lunoExchange.states[pair].BidsOrderbook[i+1:]...)
							lunoExchange.states[pair].OrderBook.Bids = append(
								lunoExchange.states[pair].OrderBook.Bids[:i],
								lunoExchange.states[pair].OrderBook.Bids[i+1:]...)
						} else {
							lunoExchange.states[pair].BidsOrderbook[i].Volume = newVolume
							lunoExchange.states[pair].OrderBook.Bids[i].Volume = newVolume
						}
						break
					}
				}
			}
		}
	}

	// Process create events
	if feedMessage.CreateUpdate != nil {
		newOrder := LunoOrderBookPriceFeed{
			Id:     feedMessage.CreateUpdate.OrderId,
			Price:  feedMessage.CreateUpdate.Price,
			Volume: feedMessage.CreateUpdate.Volume,
		}

		newPriceLevel := orderbook.PriceLevel{
			Price:  feedMessage.CreateUpdate.Price,
			Volume: feedMessage.CreateUpdate.Volume,
		}

		if feedMessage.CreateUpdate.Type == "ASK" {
			// Check if the new order is within maxPriceDiff
			if len(lunoExchange.states[pair].AsksOrderbook) > 0 {
				lowestAskPrice := lunoExchange.states[pair].AsksOrderbook[0].Price
				priceDiff := (newOrder.Price - lowestAskPrice) / lowestAskPrice
				if priceDiff <= maxPriceDiff {
					lunoExchange.states[pair].AsksOrderbook = append(
						lunoExchange.states[pair].AsksOrderbook, newOrder)
					lunoExchange.states[pair].OrderBook.Asks = append(
						lunoExchange.states[pair].OrderBook.Asks, newPriceLevel)
				}
			}
		} else {
			// Check if the new order is within maxPriceDiff
			if len(lunoExchange.states[pair].BidsOrderbook) > 0 {
				highestBidPrice := lunoExchange.states[pair].BidsOrderbook[0].Price
				priceDiff := (highestBidPrice - newOrder.Price) / highestBidPrice
				if priceDiff <= maxPriceDiff {
					lunoExchange.states[pair].BidsOrderbook = append(
						lunoExchange.states[pair].BidsOrderbook, newOrder)
					lunoExchange.states[pair].OrderBook.Bids = append(
						lunoExchange.states[pair].OrderBook.Bids, newPriceLevel)
				}
			}
		}
	}

	// Process delete events
	if feedMessage.DeleteUpdate != nil {
		// Try to find and remove from asks
		for i, ask := range lunoExchange.states[pair].AsksOrderbook {
			if ask.Id == feedMessage.DeleteUpdate.OrderId {
				lunoExchange.states[pair].AsksOrderbook = append(
					lunoExchange.states[pair].AsksOrderbook[:i],
					lunoExchange.states[pair].AsksOrderbook[i+1:]...)
				lunoExchange.states[pair].OrderBook.Asks = append(
					lunoExchange.states[pair].OrderBook.Asks[:i],
					lunoExchange.states[pair].OrderBook.Asks[i+1:]...)
				break
			}
		}
		// Try to find and remove from bids
		for i, bid := range lunoExchange.states[pair].BidsOrderbook {
			if bid.Id == feedMessage.DeleteUpdate.OrderId {
				lunoExchange.states[pair].BidsOrderbook = append(
					lunoExchange.states[pair].BidsOrderbook[:i],
					lunoExchange.states[pair].BidsOrderbook[i+1:]...)
				lunoExchange.states[pair].OrderBook.Bids = append(
					lunoExchange.states[pair].OrderBook.Bids[:i],
					lunoExchange.states[pair].OrderBook.Bids[i+1:]...)
				break
			}
		}
	}

	// Send update to subscribers
	select {
	case lunoExchange.states[pair].Updates <- lunoExchange.states[pair].OrderBook:
	default:
	}

	return nil
}
