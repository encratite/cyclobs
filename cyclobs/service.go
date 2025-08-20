package cyclobs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	// "github.com/polymarket/go-order-utils"
)

const gammaURL = "https://gamma-api.polymarket.com"
const marketsLimit = 500

type MarketsResponse struct {
	Limit int `json:"limit"`
	Count int `json:"count"`
	NextCursor string `json:"next_cursor"`
	Data []Market `json:"markets"`
}

type Market struct {
	ConditionID string `json:"condition_id"`
	QuestionID string `json:"question_id"`
	Tokens []Token `json:"tokens"`
	Rewards Rewards `json:"rewards"`
	MinimumOrderSize string `json:"minimum_order_size"`
	MinimumTickSize string `json:"minimum_tick_size"`
	Category string `json:"category"`
	EndDateISO string `json:"end_date_iso"`
	GameStartTime string `json:"game_start_time"`
	Question string `json:"question"`
	MarketSlug string `json:"market_slug"`
	MinIncentiveSize string `json:"min_incentive_size"`
	MaxIncentiveSize string `json:"max_incentive_size"`
	Active bool `json:"active"`
	Closed bool `json:"closed"`
	SecondsDelay int `json:"seconds_delay"`
	Icon string `json:"icon"`
	FPMM string `json:"fpmm"`
}

type Token struct {
	TokenID string `json:"token_id"`
	Outcome string `json:"outcome"`
}

type Rewards struct {
	MinSize float64 `json:"min_size"`
	MaxSpread float64 `json:"max_spread"`
	EventStartDate string `json:"event_start_date"`
	EventEndDate string `json:"event_end_date"`
	InGameMultiplier float64 `json:"in_game_multiplier"`
	RewardEpoch float64 `json:"reward_epoch"`
}

func RunService() {
	loadConfiguration()
	markets := getMarkets()
	if markets != nil {
		fmt.Printf("Received %d markets", len(markets))
	}
}

func getMarkets() []Market {
	offset := 0
	markets := []Market{}
	for {
		offsetString := ""
		if offset > 0 {
			offsetString = fmt.Sprintf("&offset=%d", offset)
		}
		url := fmt.Sprintf("%s/markets?limit=%d%s", gammaURL, marketsLimit, offsetString)
		response, err := http.Get(url)
		if err != nil {
			log.Printf("Failed to GET markets (%s): %v", url, err)
			return nil
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			log.Printf("Failed to read response (%s): %v", url, err)
			return nil
		}
		var responseMarkets []Market
		err = json.Unmarshal(body, &responseMarkets)
		if err != nil {
			log.Printf("Failed to parse market JSON data (%s): %v", url, err)
			log.Print(string(body))
			return nil
		}
		fmt.Printf("Loaded %d markets (%s)\n", len(responseMarkets), url)
		if len(responseMarkets) == 0 {
			break
		}
		markets = append(markets, responseMarkets...)
		offset += marketsLimit
	}
	return markets
}