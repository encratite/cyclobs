package cyclobs

import (
	"cmp"
	"fmt"
	"slices"
)

func Screener() {
	negRisk := false
	tagSlugs := []string{
		"crypto",
		"crypto-prices",
		"politics",
		"trump",
		"geopolitics",
		"hide-from-new",
	}
	priceMin := 0.7
	priceMax := 0.8
	markets := getScreenerMarkets(negRisk, tagSlugs, priceMin, priceMax)
	if markets == nil {
		return
	}
	slices.SortFunc(markets, func (a, b Market) int {
		return cmp.Compare(b.Volume1Wk, a.Volume1Wk)
	})
	fmt.Printf("Found %d matching markets:\n", len(markets))
	for i, market := range markets {
		format := "\t%d. %s: LastTradePrice = %.2f, Spread = %.2f, Volume1Wk = %.1f\n"
		fmt.Printf(format, i + 1, market.Slug, market.LastTradePrice, market.Spread, market.Volume1Wk)
	}
}

func getScreenerMarkets(negRisk bool, tagSlugs []string, priceMin, priceMax float64) []Market {
	marketMap := map[string]Market{}
	for _, tagSlug := range tagSlugs {
		events, err := getEvents(tagSlug)
		if err != nil {
			return nil
		}
		for _, event := range events {
			if event.Closed || !event.Active || event.NegRisk != negRisk {
				continue
			}
			for _, market := range event.Markets {
				if market.LastTradePrice >= priceMin && market.LastTradePrice < priceMax {
					marketMap[market.Slug] = market
				}
			}
		}
	}
	markets := []Market{}
	for _, market := range marketMap {
		markets = append(markets, market)
	}
	return markets
}