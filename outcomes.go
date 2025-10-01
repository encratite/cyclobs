package main

import (
	"fmt"

	"gonum.org/v1/gonum/stat"
)

type outcomeStats struct {
	range1 float64
	range2 float64
	prices []float64
	yesOutcomes int
}

func analyzeOutcomes() {
	analyzeOutcomesByTag("politics")
}

func analyzeOutcomesByTag(tag string) {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	closed := true
	negRisk := false
	historyData := database.getPriceHistoryData(&closed, &negRisk, nil, &tag)
	const (
		priceOffset = 24
		priceBinCount = 10
		centerRange1 = 0.45
		centerRange2 = 0.7
	)
	priceBins := []outcomeStats{}
	for i := range priceBinCount {
		range1 := float64(i) / float64(priceBinCount)
		range2 := float64(i + 1) / float64(priceBinCount)
		stats := outcomeStats{
			range1: range1,
			range2: range2,
			prices: []float64{},
			yesOutcomes: 0,
		}
		priceBins = append(priceBins, stats)
	}
	center := outcomeStats{
		range1: centerRange1,
		range2: centerRange2,
		prices: []float64{},
		yesOutcomes: 0,
	}
	for _, history := range historyData {
		if len(history.History) <= priceOffset || history.Outcome == nil {
			continue
		}
		price := history.History[priceOffset].Price
		outcome := *history.Outcome
		for i := range priceBins {
			priceBins[i].add(price, outcome)
		}
		center.add(price, outcome)
	}
	fmt.Printf("Outcome distribution for tag \"%s\":\n", tag)
	for _, stats := range priceBins {
		stats.print()
	}
	fmt.Printf("\nCenter:\n")
	center.print()
	fmt.Printf("\nTotal: %d\n", len(historyData))
}

func (s *outcomeStats) add(price float64, outcome bool) {
	if price >= s.range1 && price < s.range2 {
		s.prices = append(s.prices, price)
		if outcome {
			s.yesOutcomes++
		}
	}
}

func (s *outcomeStats) print() {
	yesRatio := float64(s.yesOutcomes) / float64(len(s.prices))
	samples := len(s.prices)
	meanPrice := stat.Mean(s.prices, nil)
	delta := meanPrice - yesRatio
	fmt.Printf("\t%.2f - %.2f: delta = %.2f, yes = %.1f%%, samples = %d, meanPrice = %.2f\n", s.range1, s.range2, delta, percent * yesRatio, samples, meanPrice)
}