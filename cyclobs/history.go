package cyclobs

import (
	"log"
	"strconv"
	"time"
)

const (
	historyPageLimit = 500
	historyMaxOffset = 10000
	historyOrder = "volumeNum"
	historyStartDateMin = "2025-06-01"
	historyFidelity = 60
	mentionMarketsTagID = 100343
)

func History() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	for offset := 0; offset < historyMaxOffset; offset += historyPageLimit {
		log.Printf("Downloading markets at offset %d", offset)
		tagID := mentionMarketsTagID
		markets, err := getMarkets(offset, historyPageLimit, historyOrder, historyStartDateMin, &tagID)
		if err != nil {
			return
		}
		if len(markets) == 0 {
			break
		}
		for _, market := range markets {
			slug := market.Slug
			exists, closed := database.priceHistoryCheck(slug)
			if exists {
				if closed {
					log.Printf("Skipping \"%s\"", slug)
					continue
				} else {
					log.Printf("Updating \"%s\"", slug)
					database.deletePriceHistory(slug)
				}
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
			startDate, err := parseISOTime(market.StartDate)
			if err != nil {
				continue
			}
			var endDatePointer *time.Time = nil
			endDate, endDateErr := parseISOTime(market.EndDate)
			if endDateErr != nil {
				endDatePointer = &endDate
			}
			tokenIDs := getCLOBTokenIDs(market)
			yesID := tokenIDs[0]
			history, err := getPriceHistory(yesID, startDate, historyFidelity)
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
				StartDate: startDate,
				EndDate: endDatePointer,
				Volume: market.VolumeNum,
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