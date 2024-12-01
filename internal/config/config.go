package config

import (
	"encoding/json"
	"os"
	"sync"
)

type Config struct {
	MarketFilter map[string]struct {
		Enabled      bool
		MaxPriceDiff float32
	}

	Luno struct {
		ApiKey    string
		ApiSecret string
	}

	Hata struct {
		ApiKey    string
		ApiSecret string
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
