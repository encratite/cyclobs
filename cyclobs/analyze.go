package cyclobs

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"gonum.org/v1/gonum/stat"
)

const (
	hoursPerDay = 24
	outcomeDistributionBins = 10
)

type outcomeCount struct {
	description string
	yes int
	no int
	total int
	prices []float64
}

type yearMonth struct {
	year int
	month time.Month
}

func Analyze() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	historyData := database.getPriceHistoryData()
	// analyzeCategories(historyData)
	// analyzeCategories(historyData)
	analyzeMonthlyDistribution(true, 0, 2, 0.4, 0.7, historyData)
}

func analyzeOutcomeDistributions(historyData []PriceHistoryBSON) {
	negRisks := []bool{
		false,
		true,
	}
	allHours := []int{
		1,
		2,
		4,
		8,
	}
	allDays := []int{
		1,
	}
	for _, negRisk := range negRisks {
		for _, hours := range allHours {
			analyzeOutcomeDistribution(negRisk, 0, hours, historyData)
		}
		for _, days := range allDays {
			analyzeOutcomeDistribution(negRisk, days, 0, historyData)
		}
	}
}

func analyzeCategories(historyData []PriceHistoryBSON) {
	outcomeMap := map[string]outcomeCount{}
	for _, history := range historyData {
		if !history.Closed || history.Outcome == nil || history.NegRisk {
			continue
		}
		for _, tag := range history.Tags {
			count, exists := outcomeMap[tag]
			if !exists {
				count = newOutcome(tag)
			}
			count.processOutcome(history)
			outcomeMap[tag] = count
		}
	}
	outcomes := []outcomeCount{}
	for _, outcome := range outcomeMap {
		outcomes = append(outcomes, outcome)
	}
	slices.SortFunc(outcomes, func (a, b outcomeCount) int {
		return cmp.Compare(a.total, b.total)
	})
	for _, outcome := range outcomes {
		fmt.Printf("%s: %s(%d samples)\n", outcome.description, outcome.getPercentage(), outcome.total)
	}
}

func analyzeOutcomeDistribution(negRisk bool, days, hours int, historyData []PriceHistoryBSON) {
	offset := days * hoursPerDay + hours
	outcomeMap := map[float64]outcomeCount{}
	for _, history := range historyData {
		if !history.Closed || history.Outcome == nil || history.NegRisk != negRisk || len(history.History) <= offset {
			continue
		}
		price := history.History[offset].Price
		key := float64(int(price * outcomeDistributionBins)) / float64(outcomeDistributionBins)
		count, exists := outcomeMap[key]
		if !exists {
			keyRange := key + 1.0 / float64(outcomeDistributionBins)
			description := fmt.Sprintf("%.1f - %.1f", key, keyRange)
			count = newOutcome(description)
		}
		count.processOutcome(history)
		outcomeMap[key] = count
	}
	outcomes := sortMap(outcomeMap, cmp.Compare)
	fmt.Printf("Distribution: negRisk = %t, days = %d, hours = %d\n", negRisk, days, hours)
	for _, outcome := range outcomes {
		fmt.Printf("%s: %s (%d samples)\n", outcome.description, outcome.getPercentage(), outcome.total)
	}
}

func analyzeMonthlyDistribution(negRisk bool, days, hours int, priceMin, priceMax float64, historyData []PriceHistoryBSON) {
	offset := days * hoursPerDay + hours
	monthMap := map[yearMonth]outcomeCount{}
	for _, history := range historyData {
		if !history.Closed || history.Outcome == nil || history.NegRisk != negRisk || len(history.History) <= offset {
			continue
		}
		sample := history.History[offset]
		if sample.Price < priceMin || sample.Price > priceMax {
			continue
		}
		key := yearMonth{
			year: sample.Timestamp.Year(),
			month: sample.Timestamp.Month(),
		}
		count, exists := monthMap[key]
		if !exists {
			monthTime := time.Date(2000, key.month, 1, 0, 0, 0, 0, time.UTC)
			monthString := monthTime.Format("Jan")
			description := fmt.Sprintf("%s %d", monthString, key.year)
			count = newOutcome(description)
		}
		count.processOutcome(history)
		count.prices = append(count.prices, sample.Price)
		monthMap[key] = count
	}
	outcomes := sortMap(monthMap, func (a, b yearMonth) int {
		if a.year != b.year {
			return cmp.Compare(a.year, b.year)
		} else {
			return cmp.Compare(a.month, b.month)
		}
	})
	for _, outcome := range outcomes {
		meanPrice := stat.Mean(outcome.prices, nil)
		yesRatio := float64(outcome.yes) / float64(outcome.total)
		delta := meanPrice - yesRatio
		fmt.Printf("%s,%.2f\n", outcome.description, delta)
	}
}

func newOutcome(description string) outcomeCount {
	return outcomeCount{
		description: description,
		yes: 0,
		no: 0,
		total: 0,
		prices: []float64{},
	}
}

func (c *outcomeCount) processOutcome(history PriceHistoryBSON) {
	if *history.Outcome {
		c.yes++
	} else {
		c.no++
	}
	c.total++
}

func (c *outcomeCount) getPercentage() string {
	percentage := float64(c.yes) / float64(c.total) * 100.0
	return fmt.Sprintf("%.1f%%", percentage)
}