package cyclobs

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	marketChannelLimit = 500
	priceChangeEvent = "price_change"
	lastTradePriceEvent = "last_trade_price"
	debugMarketChannel = false
)

func subscribeToMarkets(assetIDs []string, markets []Market) {
	if len(assetIDs) > marketChannelLimit || len(markets) > marketChannelLimit {
		log.Fatalf("Too many markets to subscribe to (%d)", len(assetIDs))
	}
	url := "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Printf("Failed to connect to market channel: %v", err)
		return
	}
	defer connection.Close()
	subscription := Subscription{
		AssetIDs: &assetIDs,
		Type: "market",
	}
	subscriptionData, err := json.Marshal(subscription)
	if err != nil {
		log.Printf("Failed to serialize subscription object: %v\n", err)
		return
	}
	err = connection.WriteMessage(websocket.TextMessage, subscriptionData)
	if err != nil {
		log.Printf("Failed to send subscription data: %v\n", err)
		return
	}
	go func () {
		for {
			pingData := []byte("PING")
			err := connection.WriteMessage(websocket.TextMessage, pingData)
			if err != nil {
				log.Printf("Failed to send ping: %v\n", err)
				break
			}
			time.Sleep(10 * time.Second)
		}
	}()
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			log.Printf("Failed to read message: %v\n", err)
			return
		}
		messageString := string(message)
		if messageString == "PONG" {
			continue
		}
		var bookMessages []BookMessage
		err = json.Unmarshal(message, &bookMessages)
		if err != nil {
			log.Printf("Failed to deserialize book message: %v\n", err)
			log.Printf("Message: %s\n", messageString)
			return
		}
		if len(bookMessages) > 1 {
			fmt.Printf("Received %d book messages\n", len(bookMessages))
		}
		for _, bookMessage := range bookMessages {
			market, exists := find(markets, func (m Market) bool {
				return m.ConditionID == bookMessage.Market
			})
			if !exists {
				continue
			}
			if bookMessage.EventType == priceChangeEvent && len(bookMessage.Changes) > 0 {
				if debugMarketChannel {
					change := bookMessage.Changes[0]
					log.Printf("Price change for market \"%s\": size = %s, price = %s, side = %s", market.Slug, change.Size, change.Price, change.Side)
				}
			} else if bookMessage.EventType == lastTradePriceEvent {
				if debugMarketChannel {
					log.Printf("Last trade price for market \"%s\": size = %s, price = %s, side = %s", market.Slug, bookMessage.Size, bookMessage.Price, bookMessage.Side)
				}
			}
		}
	}
}