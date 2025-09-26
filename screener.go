package main

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/encratite/commons"
	"github.com/fatih/color"
)

type screenerData struct {
	market Market
	tags []EventTag
}

func runScreener() {
	loadConfiguration()
	config := configuration.Jump
	markets := getScreenerMarkets(false, config.IncludeTags, config.ExcludeTags, config.Threshold2.InexactFloat64(), config.Threshold3.InexactFloat64())
	if markets == nil {
		return
	}
	slices.SortFunc(markets, func (a, b screenerData) int {
		return cmp.Compare(b.market.Volume1Wk, a.market.Volume1Wk)
	})
	fmt.Printf("Found %d matching markets:\n", len(markets))
	for i, data := range markets {
		market := data.market
		if market.Spread > config.SpreadLimit.InexactFloat64() {
			continue
		}
		tags := []string{}
		for _, tag := range data.tags {
			tags = append(tags, tag.Slug)
		}
		tagString := strings.Join(tags, ", ")
		yesID, err := getCLOBTokenID(market, true)
		if err != nil {
			continue
		}
		start := time.Now().UTC().Add(time.Duration(- 1) * time.Hour)
		history, err := getPriceHistory(yesID, start, historyFidelitySingle)
		if err != nil {
			return
		}
		if len(history.History) == 0 {
			continue
		}
		firstPrice := history.History[0]
		format := "%d. %s: firstPrice = %.2f, LastTradePrice = %.2f, Spread = %.2f, Volume1Wk = %.1f, Tags = [%s]"
		text := fmt.Sprintf(format, i + 1, market.Slug, firstPrice.Price, market.LastTradePrice, market.Spread, market.Volume1Wk, tagString)
		if firstPrice.Price <= config.Threshold1.InexactFloat64() {
			color.Green("%s\n", text)
			beep()
		} else {
			fmt.Printf("%s\n", text)
		}
	}
}

func getScreenerMarkets(negRisk bool, includeTags []string, excludeTags []string, priceMin, priceMax float64) []screenerData {
	marketMap := map[string]screenerData{}
	if len(includeTags) > 0 {
		for _, tagSlug := range includeTags {
			err := processScreenerEvents(&tagSlug, negRisk, excludeTags, priceMin, priceMax, &marketMap)
			if err != nil {
				break
			}
		}
	} else {
		_ = processScreenerEvents(nil, negRisk, excludeTags, priceMin, priceMax, &marketMap)
	}
	output := []screenerData{}
	for _, data := range marketMap {
		output = append(output, data)
	}
	return output
}

func processScreenerEvents(
	tagSlug *string,
	negRisk bool,
	excludeTags []string,
	priceMin float64,
	priceMax float64,
	marketMap *map[string]screenerData,
) error {
	events, err := getEvents(tagSlug)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Closed || !event.Active || event.NegRisk != negRisk {
			continue
		}
		excluded := false
		for _, tag := range event.Tags {
			if commons.Contains(excludeTags, tag.Slug) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
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
				(*marketMap)[market.Slug] = data
			}
		}
	}
	return nil
}