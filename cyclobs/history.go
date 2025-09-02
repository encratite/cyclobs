package cyclobs

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"
)

const (
	analysisLimit = 100
	analysisOrder = "volumeNum"
	startDateMin = "2025-01-01"
)

func History() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	offset := 0
	for {
		log.Printf("Downloading markets at offset %d", offset)
		markets, err := getMarkets(offset, analysisLimit, analysisOrder, startDateMin)
		if err != nil {
			return
		}
		if len(markets) == 0 {
			break
		}
		for _, market := range markets {
			slug := market.Slug
			exists := database.priceHistoryExists(slug)
			if exists {
				log.Printf("Skipping \"%s\"", slug)
				continue
			}
			if len(market.Events) == 0 {
				continue
			}
			event := market.Events[0]
			eventID, err := strconv.Atoi(event.ID)
			if err != nil {
				continue
			}
			eventTags, err := getEventTags(eventID)
			if err != nil {
				continue
			}
			tagSlugs := []string{}
			for _, eventTag := range eventTags {
				tagSlugs = append(tagSlugs, eventTag.Slug)
			}
			startDate, err := time.Parse(time.RFC3339, market.StartDate)
			if err != nil {
				log.Fatalf("Failed to parse timestamp: %v", err)
			}
			tokenIDs := getCLOBTokenIDs(market)
			yesID := tokenIDs[0]
			history, err := getPriceHistory(yesID, startDate)
			dbSamples := []PriceHistorySampleBSON{}
			for _, s := range history.History {
				timestamp := time.Unix(int64(s.Time), 0).UTC()
				sample := PriceHistorySampleBSON{
					Timestamp: timestamp,
					Price: s.Price,
				}
				dbSamples = append(dbSamples, sample)
			}
			outcome, err := getMarketOutcome(market)
			if err != nil {
				log.Printf("Unable to determine outcome of \"%s\": %v", slug, err)
			}
			dbHistory := PriceHistoryBSON{
				Slug: slug,
				Outcome: outcome,
				Tags: tagSlugs,
				History: dbSamples,
			}
			database.insertPriceHistory(dbHistory)
			log.Printf("Downloaded price history for \"%s\" (%d records)", slug, len(dbSamples))
		}
		offset += analysisLimit
	}
}

var marketOutcomePattern = regexp.MustCompile(`\d+`)

func getMarketOutcome(market Market) (bool, error) {
	matches := marketOutcomePattern.FindStringSubmatch(market.OutcomePrices)
	for _, match := range matches {
		switch string(match[0]) {
		case "0":
			return false, nil
		case "1":
			return true, nil
		}
	}
	return false, fmt.Errorf("Failed to parse market outcome: %s", market.OutcomePrices)
}