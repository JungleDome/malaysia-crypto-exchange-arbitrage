package config

import (
	"encoding/json"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"os"
	"sync"
)

type Config struct {
	Market map[string]struct {
		Enabled      bool
		MaxPriceDiff float32
	}

	Arbitrage map[string]struct {
		MinProfit    float32
		SlippageMode domain.SlippageDetectionModeEnum
		Slippage     float32 //percentage
	}

	Exchange map[string]struct {
		Enabled   bool
		ApiKey    string
		ApiSecret string
		MakerFee  float32
		TakerFee  float32
		Crypto    map[string]struct {
			Address           string
			Memo              string
			WithdrawFee       float32
			WithdrawMinAmount float32
			DepositMinAmount  float32
		}
	}

	Discord struct {
		WebhookUrl string
	}
}

var once sync.Once
var config *Config

func GetConfig() *Config {
	once.Do(func() {
		configBytes, err := os.ReadFile("config.json")
		if err != nil {
			panic(err)
		}
		json.Unmarshal(configBytes, &config)
	})

	return config
}
