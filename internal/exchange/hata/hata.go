package hata

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"malaysia-crypto-exchange-arbitrage/internal/platform/config"
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
var ScrapingLogger = logger.GetScrapingLogger()

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

func (exchange *HataExchange) GetTransferFee(pair string, address string, amount float32) (fee float32, err error) {
	Config := config.GetConfig()
	withdrawFee := Config.Exchange[domain.Hata.String()].Crypto

	if fee, exists := withdrawFee[pair]; exists {
		return fee.WithdrawFee, nil
	}

	return -1, nil
}

func (exchange *HataExchange) GetWithdrawMin(pair string) (min float32, err error) {
	Config := config.GetConfig()
	withdrawFee := Config.Exchange[domain.Hata.String()].Crypto

	if fee, exists := withdrawFee[pair]; exists {
		return fee.WithdrawMinAmount, nil
	}

	return 0, nil
}

func (exchange *HataExchange) GetDepositMin(pair string) (min float32, err error) {
	Config := config.GetConfig()
	depositFee := Config.Exchange[domain.Hata.String()].Crypto

	if fee, exists := depositFee[pair]; exists {
		return fee.DepositMinAmount, nil
	}

	return 0, nil
}

func (exchange *HataExchange) GetDepositAddress(pair string) (address string, err error) {
	Config := config.GetConfig()
	depositAddress := Config.Exchange[domain.Hata.String()].Crypto[pair].Address

	return depositAddress, nil
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
	} else {
		ScrapingLogger.Info(string(respBody))
	}

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
		return cmp.Compare(a.Price, b.Price)
	})

	// Sort bids by price in descending order (highest first)
	slices.SortFunc(bids, func(a, b HataOrderBookPriceFeed) int {
		return cmp.Compare(b.Price, a.Price)
	})

	Logger.Info(fmt.Sprintf("[%s] Ask: [{%f %f}] [{%f %f}] => Bid: [{%f %f}] [{%f %f}]", pair,
		asks[len(asks)-1].Price,
		asks[len(asks)-1].Volume,
		asks[0].Price,
		asks[0].Volume,
		bids[0].Price,
		bids[0].Volume,
		bids[len(bids)-1].Price,
		bids[len(bids)-1].Volume,
	))

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

	// Sort asks by price in ascending order (lowest first)
	slices.SortFunc(output.Asks, func(a, b domain.PriceLevel) int {
		return cmp.Compare(a.Price, b.Price)
	})

	// Sort bids by price in descending order (highest first)
	slices.SortFunc(output.Bids, func(a, b domain.PriceLevel) int {
		return cmp.Compare(b.Price, a.Price)
	})

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
