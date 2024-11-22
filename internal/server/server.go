package server

import (
	"github.com/gofiber/fiber/v2"

	"malaysia-crypto-exchange-arbitrage/internal/database"
)

type FiberServer struct {
	*fiber.App

	db database.Service
}

func New() *FiberServer {
	server := &FiberServer{
		App: fiber.New(fiber.Config{
			ServerHeader: "malaysia-crypto-exchange-arbitrage",
			AppName:      "malaysia-crypto-exchange-arbitrage",
		}),

		db: database.New(),
	}

	return server
}
