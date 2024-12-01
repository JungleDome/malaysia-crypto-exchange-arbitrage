package hata

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	exchange "malaysia-crypto-exchange-arbitrage/internal/exchange"
	orderbook "malaysia-crypto-exchange-arbitrage/internal/orderbook"
	logger "malaysia-crypto-exchange-arbitrage/internal/utils"
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
	state            exchange.ExchangeState
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

func (exchange *HataExchange) TestClient() (output []orderbook.PriceLevel, err error) {
	params := url.Values{}
	params.Set("pair_name", "SOLMYR")
	queryString := params.Encode()

	hmac := hmac.New(sha256.New, []byte(exchange.apiKeySecret))
	hmac.Write([]byte(queryString))
	signature := hex.EncodeToString(hmac.Sum(nil))

	Logger.Info("Signature: " + signature)

	req, err := http.NewRequestWithContext(context.Background(), "GET", exchange.apiBaseUrl+"/orderbook/api/orderbook?"+queryString, nil)
	if err != nil {
		Logger.Error("Error creating request: " + err.Error())
		return
	}
	req.Header.Set("X-API-Key", exchange.apiKeyId)
	req.Header.Set("Signature", signature)

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

	Logger.Info("Hata asks: " + fmt.Sprintf("%v", asks[len(asks)-1])) //<-- highest ask price
	Logger.Info("Hata asks: " + fmt.Sprintf("%v", asks[0]))           //<-- lowest ask price

	Logger.Info("Hata bids: " + fmt.Sprintf("%v", bids[len(bids)-1])) //<-- lowest bid price
	Logger.Info("Hata bids: " + fmt.Sprintf("%v", bids[0]))           //<-- highest bid price

	output = append(output, orderbook.PriceLevel{
		Price:  asks[0].Price,
		Volume: asks[0].Volume,
	})
	output = append(output, orderbook.PriceLevel{
		Price:  bids[0].Price,
		Volume: bids[0].Volume,
	})

	return output, nil
}

func (exchange *HataExchange) SubscribeSocket(ctx context.Context) (err error) {
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
