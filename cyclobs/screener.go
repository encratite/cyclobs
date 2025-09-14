package cyclobs

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
)

type screenerData struct {
	market Market
	tags []EventTag
}

func Screener() {
	negRisk := false
	includeTags := []string{
		"politics",
		"geopolitics",
		"world",
		"trump",
		"trump-presidency",
		"finance",
		"business",
	}
	excludeTags := []string{
		"crypto",
		"mention-markets",
		"tech",
	}
	priceMin := 0.6
	priceMax := 0.9
	markets := getScreenerMarkets(negRisk, includeTags, excludeTags, priceMin, priceMax)
	if markets == nil {
		return
	}
	slices.SortFunc(markets, func (a, b screenerData) int {
		return cmp.Compare(b.market.Volume1Wk, a.market.Volume1Wk)
	})
	fmt.Printf("Found %d matching markets:\n", len(markets))
	for i, data := range markets {
		market := data.market
		tags := []string{}
		for _, tag := range data.tags {
			tags = append(tags, tag.Slug)
		}
		tagString := strings.Join(tags, ", ")
		format := "\t%d. %s: LastTradePrice = %.2f, Spread = %.2f, Volume1Wk = %.1f, Tags = [%s]\n"
		fmt.Printf(format, i + 1, market.Slug, market.LastTradePrice, market.Spread, market.Volume1Wk, tagString)
	}
}

func getScreenerMarkets(negRisk bool, includeTags []string, excludeTags []string, priceMin, priceMax float64) []screenerData {
	marketMap := map[string]screenerData{}
	for _, tagSlug := range includeTags {
		events, err := getEvents(tagSlug)
		if err != nil {
			return nil
		}
		for _, event := range events {
			if event.Closed || !event.Active || event.NegRisk != negRisk {
				continue
			}
			excluded := false
			for _, tag := range event.Tags {
				if contains(excludeTags, tag.Slug) {
					excluded = true
					break
				}
			}
			if excluded {
				break
			}
			for _, market := range event.Markets {
				if market.LastTradePrice >= priceMin && market.LastTradePrice < priceMax {
					id, err := strconv.Atoi(event.ID)
					if err != nil {
						log.Fatalf("Unable to parse ID: %s", event.ID)
					}
					tags, err := getEventTags(id)
					if err != nil {
						log.Fatalf("Failed to get event tags: %v", err)
					}
					data := screenerData{
						market: market,
						tags: tags,
					}
					marketMap[market.Slug] = data
				}
			}
		}
	}
	output := []screenerData{}
	for _, data := range marketMap {
		output = append(output, data)
	}
	return output
}