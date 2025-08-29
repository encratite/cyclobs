package cyclobs

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const marketChannelLimit = 500

func subscribeToMarkets(assetIDs []string, callback func ([]BookMessage)) error {
	if len(assetIDs) > marketChannelLimit {
		log.Fatalf("Too many assets to subscribe to (%d)", len(assetIDs))
	}
	url := "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("Failed to connect to market channel: %v", err)
	}
	defer connection.Close()
	subscription := Subscription{
		AssetIDs: &assetIDs,
		Type: "market",
	}
	subscriptionData, err := json.Marshal(subscription)
	if err != nil {
		return fmt.Errorf("Failed to serialize subscription object: %v\n", err)
	}
	err = connection.WriteMessage(websocket.TextMessage, subscriptionData)
	if err != nil {
		return fmt.Errorf("Failed to send subscription data: %v\n", err)
	}
	go func () {
		for {
			pingData := []byte("PING")
			err := connection.WriteMessage(websocket.TextMessage, pingData)
			if err != nil {
				log.Printf("Failed to send ping: %v", err)
				break
			}
			time.Sleep(10 * time.Second)
		}
	}()
	log.Printf("Subscribed to %d markets", len(assetIDs))
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			return fmt.Errorf("Failed to read message: %v\n", err)
		}
		messageString := string(message)
		if messageString == "PONG" {
			continue
		}
		var bookMessages []BookMessage
		err = json.Unmarshal(message, &bookMessages)
		if err != nil {
			log.Printf("Failed to deserialize message: %s", messageString)
			return fmt.Errorf("Failed to deserialize book message: %v\n", err)
		}
		if len(bookMessages) > 1 {
			log.Printf("Received %d book messages", len(bookMessages))
		}
		callback(bookMessages)
	}
}