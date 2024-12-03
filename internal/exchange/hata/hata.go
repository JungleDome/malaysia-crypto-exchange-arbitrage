package hata

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"malaysia-crypto-exchange-arbitrage/internal/platform/logger"
	"net/http"
	"net/url"
	"slices"

	"github.com/coder/websocket"
)

type HataExchange struct {
	apiBaseUrl       string
	websocketBaseUrl string
	apiKeyId         string
	apiKeySecret     string
	state            domain.ExchangeState
}

type HataOrderBookPriceFeed struct {
	Price  float32 `json:"price,string"`
	Volume float32 `json:"qty,string"`
}

type HataOrderBookResponse struct {
	Data struct {
		Asks []HataOrderBookPriceFeed `json:"asks"`
		Bids []HataOrderBookPriceFeed `json:"bids"`
	} `json:"data"`
	Status string `json:"status"`
}

const hataApiBaseUrl = "https://my-api.hata.io"
const hataWebsocketBaseUrl = "wss://my-api.hata.io/orderbook/ws"

var Logger = logger.Get()

func CreateClient(id string, secret string) *HataExchange {
	exchange := HataExchange{
		apiBaseUrl:       hataApiBaseUrl,
		websocketBaseUrl: hataWebsocketBaseUrl,
		apiKeyId:         id,
		apiKeySecret:     secret,
	}

	return &exchange
}

func (exchange *HataExchange) GetName() string {
	return domain.Hata.String()
}

func (exchange *HataExchange) GetCurrentOrderBook(pair string) (output domain.OrderBook, err error) {
	params := url.Values{}
	params.Set("pair_name", pair)
	queryString := params.Encode()

	hmac := hmac.New(sha256.New, []byte(exchange.apiKeySecret))
	hmac.Write([]byte(queryString))
	signature := hex.EncodeToString(hmac.Sum(nil))

	req, err := http.NewRequestWithContext(context.Background(), "GET", exchange.apiBaseUrl+"/orderbook/api/orderbook?"+queryString, nil)
	if err != nil {
		Logger.Error("Error creating request: " + err.Error())
		return
	}
	req.Header.Set("X-API-Key", exchange.apiKeyId)
	req.Header.Set("Signature", signature)

	Logger.Info("Getting Hata order book for pair: " + pair)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		Logger.Error("Error sending request: " + err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		Logger.Error("Error reading response body: " + err.Error())
		return
	}

	// Logger.Info("Hata response: " + string(respBody))

	var respData HataOrderBookResponse
	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		Logger.Error("Error unmarshalling response body: " + err.Error())
		return
	}

	asks := respData.Data.Asks
	bids := respData.Data.Bids

	// Sort asks by price in ascending order (lowest first)
	slices.SortFunc(asks, func(a, b HataOrderBookPriceFeed) int {
		return int(a.Price) - int(b.Price)
	})

	// Sort bids by price in descending order (highest first)
	slices.SortFunc(bids, func(a, b HataOrderBookPriceFeed) int {
		return int(b.Price) - int(a.Price)
	})

	Logger.Info("Highest ask price: " + fmt.Sprintf("%v", asks[len(asks)-1])) //<-- highest ask price
	Logger.Info("Lowest ask price: " + fmt.Sprintf("%v", asks[0]))            //<-- lowest ask price

	Logger.Info("Lowest bid price: " + fmt.Sprintf("%v", bids[len(bids)-1])) //<-- lowest bid price
	Logger.Info("Highest bid price: " + fmt.Sprintf("%v", bids[0]))          //<-- highest bid price

	output.Pair = pair
	output.Exchange = domain.Hata
	output.Asks = make([]domain.PriceLevel, 0)
	output.Bids = make([]domain.PriceLevel, 0)

	for _, ask := range asks {
		output.Asks = append(output.Asks, domain.PriceLevel{
			Price:  ask.Price,
			Volume: ask.Volume,
		})
	}
	for _, bid := range bids {
		output.Bids = append(output.Bids, domain.PriceLevel{
			Price:  bid.Price,
			Volume: bid.Volume,
		})
	}

	return output, nil
}

func (exchange *HataExchange) SubscribeSocket(ctx context.Context, pair string) (err error) {
	// Create message channel and store it in the exchange struct

	c, _, err := websocket.Dial(ctx, exchange.websocketBaseUrl, nil)
	if err != nil {
		return err
	}
	defer c.CloseNow()

	// Start goroutine to read messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				c.Close(websocket.StatusNormalClosure, "")
				return
			default:
				// messageType, message, err := c.Read(ctx)
				// if err != nil {
				// 	return
				// }
				// if messageType == websocket.MessageText || messageType == websocket.MessageBinary {
				// 	// exchange.messageChannel <- message
				// }
			}
		}
	}()

	return nil
}
