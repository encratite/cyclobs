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

type Subscription struct {
	Auth *Auth `json:"auth"`
	Markets *[]string `json:"markets"`
	AssetIDs *[]string `json:"assets_ids"`
	Type string `json:"string"`
}

type Auth struct {
	APIKey string `json:"apiKey"`
	APISecret string `json:"secret"`
	PassPhrase string `json:"passphrase"`
}

type BookMessage struct {
	Market string `json:"market"`
	AssetID string `json:"asset_id"`
	Timestamp string `json:"timestamp"`
	Hash string `json:"hash"`
	Bids []OrderSummary `json:"bids"`
	Asks []OrderSummary `json:"asks"`
	Changes []PriceChange `json:"changes"`
	EventType string `json:"event_type"`
	FeeRateBPs string `json:"fee_rate_bps"`
	Price string `json:"price"`
	Side string `json:"side"`
	Size string `json:"size"`
}

type OrderSummary struct {
	Price string `json:"price"`
	Size string `json:"size"`
}

type PriceChange struct {
	Price string `json:"price"`
	Side string `json:"side"`
	Size string `json:"size"`
}

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