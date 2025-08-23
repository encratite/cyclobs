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
)

const eventsLimit = 50

func RunService() {
	loadConfiguration()
	events := getEvents("economy")
	if events != nil {
		fmt.Printf("Received %d events\n", len(events))
	}
	assetIDs := []string{}
	markets := []Market{}
	minTickSizes := map[float64]int{}
	negRisk := map[bool]int{}
	for _, event := range events {
		for _, market := range event.Markets {
			tokenIDs := getCLOBTokenIds(market)
			if len(tokenIDs) != 2 {
				// log.Printf("Invalid CLOB token ID string for market \"%s\": \"%s\"", market.Slug, market.CLOBTokenIDs)
				continue
			}
			minTickSizes[market.OrderPriceMinTickSize] += 1
			negRisk[market.NegRisk] += 1
			yesTokenID := tokenIDs[0]
			if market.Slug == "fed-decreases-interest-rates-by-25-bps-after-september-2025-meeting" {
				fmt.Printf("Token ID: %s\n", yesTokenID)
				fmt.Printf("Tick size: %.4f\n", market.OrderPriceMinTickSize)
				fmt.Printf("Neg risk: %t\n", market.NegRisk)
			}
			assetIDs = append(assetIDs, yesTokenID)
			markets = append(markets, market)
		}
	}
	for key, value := range negRisk {
		fmt.Printf("NegRisk = %t: %d\n", key, value)
	}
	// subscribeToMarkets(assetIDs, markets)
	tokenID := "56831000532202254811410354120402056896323359630546371545035370679912675847818"
	postOrder(tokenID, 5, 0.07, true, 15 * 60)
	// beep()
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