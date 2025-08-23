package cyclobs

import (
	"fmt"
	"regexp"
	"strconv"
)

const eventsLimit = 50

func RunService() {
	loadConfiguration()
	events, err := getEvents("economy")
	if err != nil {
		return
	}
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
	// tokenID := "56831000532202254811410354120402056896323359630546371545035370679912675847818"
	// postOrder(tokenID, 5, 0.07, true, 15 * 60)
	// beep()
	positions, _ := getPositions()
	fmt.Printf("Retrieved %d positions\n", len(positions))
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

func getEvents(tagSlug string) ([]Event, error) {
	url := "https://gamma-api.polymarket.com/events/pagination"
	parameters := map[string]string{
		"limit": strconv.FormatInt(eventsLimit, 10),
		"archived": "false",
		"tag_slug": tagSlug,
		"order": "volume24hr",
		"ascending": "false",
	}
	events, err := getJSON[EventsResponse](url, parameters)
	if err != nil {
		return nil, err
	}
	return events.Data, nil
}

func getPositions() ([]Position, error) {
	url := "https://data-api.polymarket.com/positions"
	parameters := map[string]string{
		"user": configuration.Credentials.ProxyAddress,
	}
	positions, err := getJSON[[]Position](url, parameters)
	if err != nil {
		return nil, err
	}
	return positions, nil
}