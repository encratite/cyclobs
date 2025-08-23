package cyclobs

import (
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/polymarket/go-order-utils/pkg/model"
)

const eventsLimit = 50

type positionState struct {
	size float64
	added time.Time
}

func RunService() {
	loadConfiguration()
	go runCleaner()
	events, err := getEvents("economy")
	if err != nil {
		return
	}
	assetIDs := []string{}
	markets := []Market{}
	minTickSizes := map[float64]int{}
	negRisk := map[bool]int{}
	for _, event := range events {
		for _, market := range event.Markets {
			tokenIDs := getCLOBTokenIds(market)
			if len(tokenIDs) != 2 {
				continue
			}
			minTickSizes[market.OrderPriceMinTickSize] += 1
			negRisk[market.NegRisk] += 1
			yesTokenID := tokenIDs[0]
			assetIDs = append(assetIDs, yesTokenID)
			markets = append(markets, market)
		}
	}
	// subscribeToMarkets(assetIDs, markets)
	// tokenID := "56831000532202254811410354120402056896323359630546371545035370679912675847818"
	// postOrder(tokenID, model.BUY, 5, 0.07, true, 15 * 60)
	// beep()
}

func runCleaner() {
	cleanerConfig := configuration.Cleaner
	states := map[string]positionState{}
	sleep := func() {
		duration := time.Duration(*cleanerConfig.Interval) * time.Second
		time.Sleep(duration)
	}
	for {
		positions, err := getPositions()
		if err != nil {
			sleep()
			continue
		}
		now := time.Now()
		positionExpiration := time.Duration(*cleanerConfig.Expiration) * time.Second
		for _, position := range positions {
			state, exists := states[position.Asset]
			if !exists {
				state = positionState{
					size: position.Size,
					added: now,
				}
				states[position.Asset] = state
				log.Printf("Detected new position in cleaner: slug = %s, size = %.2f, added = %s\n", position.Slug, position.Size, now)
				continue
			}
			if state.size != position.Size {
				log.Printf("Size of position %s has changed from %.2f to %.2f, resetting expiration\n", position.Slug, state.size, position.Size)
				states[position.Asset] = positionState{
					size: position.Size,
					added: now,
				}
				continue
			}
			age := now.Sub(state.added)
			if age < positionExpiration {
				continue
			}
			log.Printf("Position %s has expired, closing it\n", position.Slug)
			size := int(position.Size)
			limit := max(position.CurPrice - *cleanerConfig.Tolerance, 0.05)
			orderExpiration := *cleanerConfig.Interval
			postOrder(position.Asset, model.SELL, size, limit, position.NegativeRisk, orderExpiration)
		}
		sleep()
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