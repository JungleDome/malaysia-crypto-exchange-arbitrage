package luno

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"malaysia-crypto-exchange-arbitrage/internal/platform/config"
	"malaysia-crypto-exchange-arbitrage/internal/platform/logger"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/luno/luno-go"
	"github.com/luno/luno-go/decimal"
)

type LunoExchange struct {
	lunoClient       luno.Client
	websocketBaseUrl string
	apiKeyId         string
	apiKeySecret     string
	states           map[string]*LunoExchangeState
}

type LunoExchangeState struct {
	domain.ExchangeState
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

func (lunoExchange *LunoExchange) GetName() string {
	return domain.Luno.String()
}

func (lunoExchange *LunoExchange) GetTransferFee(pair string, amount float32) float32 {
	res, err := lunoExchange.lunoClient.SendFee(context.Background(), &luno.SendFeeRequest{Address: "", Amount: decimal.NewFromFloat64(float64(amount), -8)})
	if err != nil {
		Logger.Error("Failed to get Luno transfer fee: " + err.Error())
		return 0
	}
	return float32(res.Fee.Float64())
}

func (lunoExchange *LunoExchange) GetCurrentOrderBook(pair string) (output domain.OrderBook, err error) {
	req := luno.GetOrderBookRequest{Pair: pair}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()

	Logger.Info("Getting Luno order book for pair: " + req.Pair)

	res, err := lunoExchange.lunoClient.GetOrderBook(ctx, &req)
	if err != nil {
		log.Printf("Failed to get Luno order book: %v", err)
	}
	// log.Println(res)

	Logger.Info("Highest ask price: " + fmt.Sprintf("%v", res.Asks[len(res.Asks)-1])) //<-- highest ask price
	Logger.Info("Lowest ask price: " + fmt.Sprintf("%v", res.Asks[0]))                //<-- lowest ask price

	Logger.Info("Lowest bid price: " + fmt.Sprintf("%v", res.Bids[len(res.Bids)-1])) //<-- lowest bid price
	Logger.Info("Highest bid price: " + fmt.Sprintf("%v", res.Bids[0]))              //<-- highest bid price

	output.Pair = pair
	output.Exchange = domain.Luno
	output.Asks = make([]domain.PriceLevel, 0)
	output.Bids = make([]domain.PriceLevel, 0)

	for _, ask := range res.Asks {
		output.Asks = append(output.Asks, domain.PriceLevel{
			Price:  float32(ask.Price.Float64()),
			Volume: float32(ask.Volume.Float64()),
		})
	}
	for _, bid := range res.Bids {
		output.Bids = append(output.Bids, domain.PriceLevel{
			Price:  float32(bid.Price.Float64()),
			Volume: float32(bid.Volume.Float64()),
		})
	}

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
					err := lunoExchange.processOrderBookFeed(ctx, message, pair)
					if err != nil {
						if errors.As(err, &SequenceIncorrectError{}) {
							Logger.Error("Sequence number mismatch. Expected: " + fmt.Sprintf("%d", err.(*SequenceIncorrectError).ExpectedSequence) + ", got: " + fmt.Sprintf("%d", err.(*SequenceIncorrectError).ActualSequence))
							lunoExchange.resubscribeSocket(ctx, c, pair)
						} else {
							Logger.Error("Failed to process Luno order book feed: " + err.Error())
						}
					}
				} else {
					Logger.Error("Received unknown message type from Luno websocket: " + strconv.Itoa(int(messageType)))
				}
			}
		}
	}()

	return
}

func (lunoExchange *LunoExchange) resubscribeSocket(ctx context.Context, c *websocket.Conn, pair string) (err error) {
	Logger.Info("Resubscribing to Luno websocket for pair: " + pair)

	// Close current connection
	c.Close(websocket.StatusNormalClosure, "")

	// Reset state
	lunoExchange.states[pair].Mutex.Lock()
	defer lunoExchange.states[pair].Mutex.Unlock()
	lunoExchange.states[pair] = nil

	return lunoExchange.SubscribeSocket(ctx, pair)
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

func (lunoExchange *LunoExchange) processOrderBookFeed(ctx context.Context, feedString []byte, pair string) error {
	config := config.GetConfig()
	maxPriceDiff := config.Market[pair].MaxPriceDiff

	if lunoExchange.states[pair] == nil {
		var feedSnapshot *LunoOrderBookFeedSnapshot
		err := json.Unmarshal(feedString, &feedSnapshot)
		if err != nil {
			return fmt.Errorf("failed to unmarshal Luno order book feed snapshot: %v", err)
		}

		lunoExchange.states[pair] = &LunoExchangeState{
			ExchangeState: domain.ExchangeState{
				OrderBook: &domain.OrderBook{Pair: pair},
				Updates:   make(chan *domain.OrderBook),
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

		err := lunoExchange.ProcessSequenceNumber(pair, feedMessage.Sequence)
		if err != nil {
			return err
		}

		err = lunoExchange.processFeedUpdate(feedMessage, pair, maxPriceDiff)
		StateLogger.Info("Current internal state for pair: " + pair + " is: " + fmt.Sprintf("%v", lunoExchange.states[pair]))
		if err != nil {
			return err
		}
	}

	return nil
}

func (lunoExchange *LunoExchange) processFeedSnapshot(feedSnapshot *LunoOrderBookFeedSnapshot, pair string, maxPriceDiff float32) error {
	// Process asks
	asks := make([]domain.PriceLevel, 0)
	StateLogger.Info("Feed snapshot for pair: " + pair + " is: " + fmt.Sprintf("%v", feedSnapshot))
	lowestAskPrice := feedSnapshot.Asks[0].Price // First ask is lowest
	for _, ask := range feedSnapshot.Asks {
		// Only include asks within maxPriceDiff of lowest ask
		priceDiff := (ask.Price - lowestAskPrice) / lowestAskPrice
		if priceDiff <= maxPriceDiff {
			asks = append(asks, domain.PriceLevel{
				Price:  ask.Price,
				Volume: ask.Volume,
			})
			lunoExchange.states[pair].AsksOrderbook = append(lunoExchange.states[pair].AsksOrderbook, ask)
		}
	}

	// Process bids
	bids := make([]domain.PriceLevel, 0)
	highestBidPrice := feedSnapshot.Bids[0].Price // First bid is highest
	for _, bid := range feedSnapshot.Bids {
		// Only include bids within maxPriceDiff of highest bid
		priceDiff := (highestBidPrice - bid.Price) / highestBidPrice
		if priceDiff <= maxPriceDiff {
			bids = append(bids, domain.PriceLevel{
				Price:  bid.Price,
				Volume: bid.Volume,
			})
			lunoExchange.states[pair].BidsOrderbook = append(lunoExchange.states[pair].BidsOrderbook, bid)
		}
	}

	lunoExchange.states[pair].OrderBook = &domain.OrderBook{
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

		newPriceLevel := domain.PriceLevel{
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

func (lunoExchange *LunoExchange) ProcessSequenceNumber(pair string, sequence int) error {
	previousSequence := lunoExchange.states[pair].CurrentSequence
	if sequence == previousSequence+1 {
		lunoExchange.states[pair].CurrentSequence = sequence
	} else {
		return &SequenceIncorrectError{
			ExpectedSequence: previousSequence + 1,
			ActualSequence:   sequence,
		}
	}

	return nil
}
