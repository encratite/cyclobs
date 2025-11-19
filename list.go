package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/encratite/commons"
	"github.com/encratite/gamma"
	"gonum.org/v1/gonum/stat"
)

const (
	listMarketsOrder = "startDate"
	listMarketsFirstDate = "2024-01-01"
	listMarketsFinalPriceOffset = 500
	listMarketsLowerLimit = 0.015
	listMarketsUpperLimit = 0.985
)

var listMarketsIgnore []string = []string{
	"mrbeast",
}

type listOutcomeBin struct {
	priceRange1 float64
	priceRange2 float64
	yes int
	no int
	prices []float64
}

func listMarkets(slug string, directory string) {
	tag, err := gamma.GetTag(slug)
	if err != nil {
		log.Fatalf("Failed to retrieve tag: %v", err)
	}
	tagID := commons.MustParseInt(tag.ID)
	markets := []gamma.Market{}
	for offset := 0;; offset += historyPageLimit {
		marketBatch, err := gamma.GetMarkets(offset, historyPageLimit, listMarketsOrder, listMarketsFirstDate, &tagID)
		if err != nil {
			log.Fatalf("Failed to get markets: %v", err)
		}
		for _, market := range marketBatch {
			if !market.Closed {
				continue
			}
			ignored := false
			for _, ignore := range listMarketsIgnore {
				if strings.Contains(market.Slug, ignore) {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			outcome := getMarketOutcome(market)
			if outcome == nil {
				continue
			}
			downloadEvent(market.Slug, directory)
			markets = append(markets, market)
		}
		if len(marketBatch) < historyPageLimit {
			break
		}
	}
	counter := 1
	yes := 0
	yesPrices := []float64{}
	noPrices := []float64{}
	outcomeBins := []listOutcomeBin{
		{
			priceRange1: 0.00,
			priceRange2: 0.30,
		},
		{
			priceRange1: 0.30,
			priceRange2: 0.60,
		},
		{
			priceRange1: 0.60,
			priceRange2: 0.80,
		},
		{
			priceRange1: 0.80,
			priceRange2: 0.90,
		},
		{
			priceRange1: 0.90,
			priceRange2: 1.00,
		},
	}
	for _, market := range markets {
		outcome := getMarketOutcome(market)
		path := filepath.Join(directory, fmt.Sprintf("%s.csv", market.Slug))
		prices := []float64{}
		commons.ReadCSV(path, func (records []string) {
			price, err := commons.ParseFloat(records[1])
			if err != nil {
				return
			}
			prices = append(prices, price)
		})
		finalPrice := getFinalPrice(prices)
		finalPriceString := "?"
		if finalPrice != nil {
			finalPriceString = fmt.Sprintf("%.2fc", *finalPrice)
			if *outcome {
				yesPrices = append(yesPrices, *finalPrice)
			} else {
				noPrices = append(noPrices, *finalPrice)
			}
			for i := range outcomeBins {
				bin := &outcomeBins[i]
				if *finalPrice >= bin.priceRange1 && *finalPrice < bin.priceRange2 {
					if *outcome {
						bin.yes++
					} else {
						bin.no++
					}
					bin.prices = append(bin.prices, *finalPrice)
				}
			}
		}
		fmt.Printf("%d. %s: %t (%s)\n", counter, market.Slug, *outcome, finalPriceString)
		if *outcome {
			yes++
		}
		counter++
	}
	yesRatio := percent * float64(yes) / float64(counter)
	yesMeanPrice := stat.Mean(yesPrices, nil)
	noMeanPrice := stat.Mean(noPrices, nil)
	fmt.Printf("\nMarkets that resolve to \"yes\": %.1f%%\n", yesRatio)
	fmt.Printf("Mean price of \"yes\" markets: %.2fc\n", yesMeanPrice)
	fmt.Printf("Mean price of \"no\" markets: %.2fc\n\n", noMeanPrice)
	for _, bin := range outcomeBins {
		meanPrice := stat.Mean(bin.prices, nil)
		fmt.Printf("%.2f - %.2f: %.2fc (%d samples)\n", bin.priceRange1, bin.priceRange2, meanPrice, len(bin.prices))
	}
}

func getFinalPrice(prices []float64) *float64 {
	for i, price := range prices {
		if price < listMarketsLowerLimit || price > listMarketsUpperLimit {
			offset := max(i - listMarketsFinalPriceOffset, 0)
			finalPrice := prices[offset]
			return &finalPrice
		}
	}
	return nil
}