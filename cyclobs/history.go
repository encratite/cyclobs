package cyclobs

import (
	"log"
	"strconv"
	"time"
)

const (
	historyPageLimit = 100
	historyMaxOffset = 2000
	historyOrder = "volumeNum"
	historyStartDateMin = "2025-01-01"
)

func History() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	for offset := 0; offset < historyMaxOffset; offset += historyPageLimit {
		log.Printf("Downloading markets at offset %d", offset)
		markets, err := getMarkets(offset, historyPageLimit, historyOrder, historyStartDateMin)
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
			outcome := getMarketOutcome(market)
			dbHistory := PriceHistoryBSON{
				Slug: slug,
				NegRisk: market.NegRisk,
				Closed: market.Closed,
				Outcome: outcome,
				Tags: tagSlugs,
				History: dbSamples,
			}
			database.insertPriceHistory(dbHistory)
			log.Printf("Downloaded price history for \"%s\" (%d records)", slug, len(dbSamples))
		}
	}
}

func getMarketOutcome(market Market) *bool {
	var outcome bool
	switch market.OutcomePrices {
	case "[\"0\", \"1\"]":
		outcome = false
		return &outcome
	case "[\"1\", \"0\"]":
		outcome = true
		return &outcome
	}
	return nil
}