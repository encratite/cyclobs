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
	seasonalityPriceMin = 0.01
	seasonalityPriceMax = 0.98
	seasonalityMinYear = 2000
	minHistorySize = 10
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

type hourReturns struct {
	hour int
	returns []float64
}

type weekdayReturns struct {
	day time.Weekday
	returns []float64
}

type priceRangeReturns struct {
	rangeMin float64
	rangeMax float64
	deltas []float64
}

func Analyze() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	historyData := database.getPriceHistoryData()
	// analyzeCategories(historyData)
	// analyzeCategories(historyData)
	// analyzeMonthlyDistribution(false, 0, 1, 0.4, 0.6, historyData)
	// analyzeHours(historyData)
	analyzePriceRanges(historyData)
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

func analyzeHours(historyData []PriceHistoryBSON) {
	negRisks := []bool{
		false,
		true,
	}
	samplingHours := []int{
		8, 12, 15, 18, 20,
	}
	for _, negRisk := range negRisks {
		for _, samplingHour := range samplingHours {
			analyzeWeekdaySeasonality(negRisk, samplingHour, historyData)
		}
	}
}

func analyzeHourSeasonality(negRisk bool, historyData []PriceHistoryBSON) {
	hourMap := map[int]hourReturns{}
	for _, history := range historyData {
		if history.NegRisk != negRisk {
			continue
		}
		for i := range history.History {
			if i == 0 {
				continue
			}
			sample1 := history.History[i - 1]
			sample2 := history.History[i]
			price1 := sample1.Price
			price2 := sample2.Price
			if !includePrices(price1, price2) {
				continue
			}
			returns := getReturns(price2, price1)
			key := sample2.Timestamp.Hour()
			entry, exists := hourMap[key]
			if !exists {
				entry = hourReturns{
					hour: key,
					returns: []float64{},
				}
			}
			entry.returns = append(entry.returns, returns)
			hourMap[key] = entry
		}
	}
	entries := sortMap(hourMap, cmp.Compare)
	fmt.Printf("Seasonality by hour (negRisk = %t)\n", negRisk)
	for _, entry := range entries {
		meanReturn := stat.Mean(entry.returns, nil)
		fmt.Printf("\t%dh: %.2f%%\n", entry.hour, meanReturn * 100)
	}
	fmt.Printf("\n")
}

func analyzeWeekdaySeasonality(negRisk bool, samplingHour int, historyData []PriceHistoryBSON) {
	weekdayMap := map[time.Weekday]weekdayReturns{}
	for _, history := range historyData {
		if history.NegRisk != negRisk || len(history.History) < minHistorySize {
			continue
		}
		samples := getSamplingHours(negRisk, samplingHour, history)
		for i, sample := range samples {
			if i == 0 {
				continue
			}
			previousSample := samples[i - 1]
			timestamp := sample.Timestamp
			date := getDate(timestamp)
			if date.Year() < seasonalityMinYear {
				continue
			}
			key := sample.Timestamp.Weekday()
			price1 := previousSample.Price
			price2 := sample.Price
			if !includePrices(price1, price2) {
				continue
			}
			returns := getReturns(price2, price1)
			entry, exists := weekdayMap[key]
			if !exists {
				entry = weekdayReturns{
					day: key,
					returns: []float64{},
				}
			}
			entry.returns = append(entry.returns, returns)
			weekdayMap[key] = entry
		}
	}
	entries := sortMap(weekdayMap, cmp.Compare)
	fmt.Printf("Seasonality by day (negRisk = %t, samplingHour = %dh):\n", negRisk, samplingHour)
	for _, entry := range entries {
		meanReturns := stat.Mean(entry.returns, nil)
		fmt.Printf("\t%s: %.2f%%\n", entry.day, meanReturns * 100.0)
	}
	fmt.Printf("\n")
}

func analyzePriceRanges(historyData []PriceHistoryBSON) {
	negRisks := []bool{
		false,
		true,
	}
	samplingHours := []int{
		15,
	}
	offsets := []int{
		2, 7, 14,
	}
	for _, negRisk := range negRisks {
		for _, samplingHour := range samplingHours {
			for _, offset := range offsets {
				analyzePriceRangeReturns(negRisk, samplingHour, offset, historyData)
			}
		}
	}
}

func analyzePriceRangeReturns(negRisk bool, samplingHour int, offset int, historyData []PriceHistoryBSON) {
	priceRangeBins := []priceRangeReturns{
		{
			rangeMin: 0.0,
			rangeMax: 0.02,
		},
		{
			rangeMin: 0.02,
			rangeMax: 0.1,
		},
	}
	for i := 0.1; i < 0.9; i += 0.1 {
		priceRangeBins = append(priceRangeBins, priceRangeReturns{
			rangeMin: i,
			rangeMax: i + 0.1,
		})
	}
	priceRangeBins = append(priceRangeBins, priceRangeReturns{
		rangeMin: 0.9,
		rangeMax: 0.98,
	})
	priceRangeBins = append(priceRangeBins, priceRangeReturns{
		rangeMin: 0.98,
		rangeMax: 1.0,
	})
	for _, history := range historyData {
		if history.NegRisk != negRisk || len(history.History) < minHistorySize {
			continue
		}
		samples := getSamplingHours(negRisk, samplingHour, history)
		for i := range (len(samples) - offset) {
			price1 := samples[i].Price
			price2 := samples[i + offset].Price
			delta := price2 - price1
			for j := range priceRangeBins {
				priceRange := &priceRangeBins[j]
				if price1 >= priceRange.rangeMin && price2 < priceRange.rangeMax {
					priceRange.deltas = append(priceRange.deltas, delta)
				}
			}
		}
	}
	fmt.Printf("Returns by price range (negRisk = %t, samplingHour = %dh, offset = %d):\n", negRisk, samplingHour, offset)
	for _, priceRange := range priceRangeBins {
		meanDelta := stat.Mean(priceRange.deltas, nil)
		fmt.Printf("\t%.2f - %.2f: %.3f\n", priceRange.rangeMin, priceRange.rangeMax, meanDelta)
	}
	fmt.Printf("\n")
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

func includePrices(price1, price2 float64) bool {
	match1 := price1 >= seasonalityPriceMin && price1 <= seasonalityPriceMax
	match2 := price2 >= seasonalityPriceMin && price2 <= seasonalityPriceMax
	return match1 && match2
}

func getSamplingHours(negRisk bool, samplingHour int, history PriceHistoryBSON) []PriceHistorySampleBSON {
	samples := []PriceHistorySampleBSON{}
	previousDate := time.Time{}
	hasPreviousSample := false
	for _, sample := range history.History {
		timestamp := sample.Timestamp
		date := getDate(timestamp)
		if date.Year() < seasonalityMinYear {
			continue
		}
		if !hasPreviousSample {
			previousDate = date
			hasPreviousSample = true
			continue
		}
		if timestamp.Hour() >= samplingHour && date != previousDate {
			samples = append(samples, sample)
			previousDate = date
		}
	}
	return samples
}