package cyclobs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const eventsLimit = 50

func RunService() {
	loadConfiguration()
	events := getEvents("politics")
	if events != nil {
		fmt.Printf("Received %d events\n", len(events))
	}
	assetIDs := []string{}
	markets := []Market{}
	for _, event := range events {
		for _, market := range event.Markets {
			tokenIDs := getCLOBTokenIds(market)
			if len(tokenIDs) != 2 {
				// log.Printf("Invalid CLOB token ID string for market \"%s\": \"%s\"", market.Slug, market.CLOBTokenIDs)
				continue
			}
			yesTokenID := tokenIDs[0]
			assetIDs = append(assetIDs, yesTokenID)
			markets = append(markets, market)
		}
	}
	subscribeToMarkets(assetIDs, markets)
}

func getEvents(tagSlug string) []Event {
	base := "https://gamma-api.polymarket.com/events/pagination"
	u, err := url.Parse(base)
	if err != nil {
		log.Fatalf("Unable to parse URL (%s): %v", base, err)
	}
	values := url.Values{}
	values.Add("limit", strconv.FormatInt(eventsLimit, 10))
	values.Add("archived", "false")
	values.Add("tag_slug", tagSlug)
	values.Add("order", "volume24hr")
	values.Add("ascending", "false")
	u.RawQuery = values.Encode()
	encoded := u.String()
	response, err := http.Get(encoded)
	if err != nil {
		log.Printf("Failed to GET markets (%s): %v", encoded, err)
		return nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response (%s): %v", encoded, err)
		return nil
	}
	var eventsResponse EventsResponse
	err = json.Unmarshal(body, &eventsResponse)
	if err != nil {
		log.Printf("Failed to parse market JSON data (%s): %v", encoded, err)
		return nil
	}
	return eventsResponse.Data
}

func subscribeToMarkets(assetIDs []string, markets []Market) {
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
			if len(bookMessage.Changes) > 0 {
				market, exists := find(markets, func (m Market) bool {
					return m.ConditionID == bookMessage.Market
				})
				if exists {
					change := bookMessage.Changes[0]
					log.Printf("Price change for market \"%s\": %s x $%s (%s)", market.Slug, change.Size, change.Price, change.Side)
				}
			}
		}
	}
}

var clobTokenIdPattern = regexp.MustCompile(`\d+`)

func getCLOBTokenIds(market Market) []string {
	tokenIds := []string{}
	matches := clobTokenIdPattern.FindAllStringSubmatch(market.CLOBTokenIDs, -1)
	for _, match := range matches {
		tokenId := match[0]
		tokenIds = append(tokenIds, tokenId)
	}
	return tokenIds
}