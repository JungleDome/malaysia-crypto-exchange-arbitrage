package arbitrage

import (
	"context"
	"fmt"
	"malaysia-crypto-exchange-arbitrage/internal/domain"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/webhook"
)

func AlertDiscord(arbitrageOpportunity domain.ArbitrageOpportunity) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := webhook.NewWithURL(Config.Discord.WebhookUrl)
	if err != nil {
		Logger.Error("Failed to create discord session: " + err.Error())
		return
	}
	defer client.Close(ctx)

	_, err = client.CreateEmbeds([]discord.Embed{
		discord.NewEmbedBuilder().
			SetTitle("Arbitrage opportunities found").
			SetColor(0x00ff00).
			AddField("Buy On", arbitrageOpportunity.BuyOn, true).
			AddField("Sell On", arbitrageOpportunity.SellOn, true).
			AddField("Pair", arbitrageOpportunity.Pair, true).
			AddField("\u200B", "\u200B", false).
			AddField("Buy Price", fmt.Sprintf("%f", arbitrageOpportunity.BuyPrice), true).
			AddField("Buy Volume", fmt.Sprintf("%f", arbitrageOpportunity.BuyVolume), true).
			AddField("Total Buy Price", fmt.Sprintf("%f", arbitrageOpportunity.TotalBuyPrice), true).
			AddField("Sell Price", fmt.Sprintf("%f", arbitrageOpportunity.SellPrice), true).
			AddField("Sell Volume", fmt.Sprintf("%f", arbitrageOpportunity.SellVolume), true).
			AddField("Total Sell Price", fmt.Sprintf("%f", arbitrageOpportunity.TotalSellPrice), true).
			AddField("\u200B", "\u200B", false).
			AddField("Buy Fee", fmt.Sprintf("%f", arbitrageOpportunity.BuyFee), true).
			AddField("Sell Fee", fmt.Sprintf("%f", arbitrageOpportunity.SellFee), true).
			AddField("Net Profit", fmt.Sprintf("%f", arbitrageOpportunity.NetProfit), true).
			Build()})
	if err != nil {
		Logger.Error("Failed to send message to discord: " + err.Error())
	}
}
