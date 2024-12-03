package main

import (
	"context"
	"fmt"
	"log"
	"malaysia-crypto-exchange-arbitrage/internal/arbitrage"
	"malaysia-crypto-exchange-arbitrage/internal/config"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"malaysia-crypto-exchange-arbitrage/internal/exchange/hata"
	"malaysia-crypto-exchange-arbitrage/internal/exchange/luno"
	"malaysia-crypto-exchange-arbitrage/internal/server"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

func gracefulShutdown(fiberServer *server.FiberServer, done chan bool) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Listen for the interrupt signal.
	<-ctx.Done()

	log.Println("shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fiberServer.ShutdownWithContext(ctx); err != nil {
		log.Printf("Server forced to shutdown with error: %v", err)
	}

	log.Println("Server exiting")

	// Notify the main goroutine that the shutdown is complete
	done <- true
}

func main() {

	debug := true
	if debug {
		ctx, cancel := context.WithCancel(context.Background())

		config := config.GetConfig()

		exchanges := []domain.Exchanger{}

		var ex domain.Exchanger = luno.CreateClient(config.Exchange["Luno"].ApiKey, config.Exchange["Luno"].ApiSecret)
		exchanges = append(exchanges, ex)
		var ex2 domain.Exchanger = hata.CreateClient(config.Exchange["Hata"].ApiKey, config.Exchange["Hata"].ApiSecret)
		exchanges = append(exchanges, ex2)

		pairs := make([]string, 0)
		for pair := range config.Market {
			if config.Market[pair].Enabled {
				pairs = append(pairs, pair)
			}
		}

		watcher := arbitrage.NewArbitrageScheduledWatcher(ctx, exchanges, pairs, 30*time.Second, domain.Scheduled)
		watcher.Start()

		// for pair := range config.Market {
		// 	if config.Market[pair].Enabled {
		// 		go arbitrage.StartWatching(ctx, pair, []domain.Exchanger{ex, ex2}, 30*time.Second)
		// 	}
		// }

		// Set up a channel to handle system interrupts (Ctrl+C)
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

		// Block until an interrupt signal is received
		select {
		case <-interrupt:
			cancel()
			fmt.Println("Interrupt signal received. Shutting down...")
		}
	} else {
		server := server.New()

		server.RegisterFiberRoutes()

		// Create a done channel to signal when the shutdown is complete
		done := make(chan bool, 1)

		go func() {
			port, _ := strconv.Atoi(os.Getenv("PORT"))
			err := server.Listen(fmt.Sprintf(":%d", port))
			if err != nil {
				panic(fmt.Sprintf("http server error: %s", err))
			}
		}()

		// Run graceful shutdown in a separate goroutine
		go gracefulShutdown(server, done)

		// Wait for the graceful shutdown to complete
		<-done
		log.Println("Graceful shutdown complete.")
	}
}
