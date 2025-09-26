package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/encratite/commons"
	"gonum.org/v1/gonum/stat"
)

const (
	hoursPerDay = 24
	categoryLimit = 20
	outcomeDistributionBins = 10
	seasonalityPriceMin = 0.01
	seasonalityPriceMax = 0.98
	minHistorySize = 10
	quantileCount = 5
	dataMinYear = 2000
	tagSamplesMin = 20
	priceRangeBinsDaysMin = 2
	spread = 0.01
	percent = 100.0
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
	yesReturns []float64
	noReturns []float64
	hit bool
}

type categoryData struct {
	category string
	outcome outcomeCount
	quantiles []quantileData
}

type quantileData struct {
	prices []float64
}

type tagDeltaData struct {
	tag string
	delta float64
	yes float64
	no float64
	samples int
}

func analyzeData() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	closed := true
	historyData := database.getPriceHistoryData(&closed, nil, nil, nil)
	// analyzeCategories(false, historyData)
	// analyzeCategories(true, historyData)
	// analyzeMonthlyDistribution(false, 0, 1, 0.4, 0.6, historyData)
	// analyzeHours(historyData)
	// analyzePriceRanges(historyData)
	analyzeCategoryPriceRanges(historyData)
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

func analyzeCategories(negRisk bool, historyData []PriceHistoryBSON) {
	categoryMap := map[string]categoryData{}
	for _, history := range historyData {
		if !history.Closed || history.Outcome == nil || history.NegRisk != negRisk || len(history.History) < quantileCount {
			continue
		}
		if history.StartDate.Year() < dataMinYear {
			continue
		}
		quantilePrices := make([]float64, 0, quantileCount)
		for i := range quantileCount {
			index := 1 + i * len(history.History) / (quantileCount + 1)
			price := history.History[index].Price
			quantilePrices = append(quantilePrices, price)
		}
		for _, tag := range history.Tags {
			data, exists := categoryMap[tag]
			if !exists {
				quantiles := make([]quantileData, 0, quantileCount)
				for range quantilePrices {
					d := quantileData{
						prices: []float64{},
					}
					quantiles = append(quantiles, d)
				}
				data = categoryData{
					category: tag,
					outcome: newOutcome(tag),
					quantiles: quantiles,
				}

			}
			data.outcome.processOutcome(history)
			for i, price := range quantilePrices {
				destination := &data.quantiles[i].prices
				*destination = append(*destination, price)
			}
			categoryMap[tag] = data
		}
	}
	categories := sortMapByValue(categoryMap, func (a, b categoryData) int {
		return cmp.Compare(b.outcome.total, a.outcome.total)
	})
	fmt.Printf("Categories (negRisk = %t):\n", negRisk)
	for i, category := range categories {
		if i >= categoryLimit {
			break
		}
		quantileStrings := []string{}
		for _, quantileData := range category.quantiles {
			meanPrice := stat.Mean(quantileData.prices, nil)
			quantileStrings = append(quantileStrings, fmt.Sprintf("%.2f", meanPrice))
		}
		quantileString := strings.Join(quantileStrings, ", ")
		fmt.Printf("\t%d. %s: %s - %s (%d samples)\n", i + 1, category.outcome.description, category.outcome.getPercentage(), quantileString, category.outcome.total)
	}
	fmt.Printf("\n")
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
	outcomes := sortMapByKey(outcomeMap, cmp.Compare)
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
	outcomes := sortMapByKey(monthMap, func (a, b yearMonth) int {
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
			returns := getRateOfChange(price2, price1)
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
	entries := sortMapByKey(hourMap, cmp.Compare)
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
		samples := getSamplingHours(samplingHour, history)
		for i, sample := range samples {
			if i == 0 {
				continue
			}
			previousSample := samples[i - 1]
			timestamp := sample.Timestamp
			date := commons.GetDate(timestamp)
			if date.Year() < dataMinYear {
				continue
			}
			key := sample.Timestamp.Weekday()
			price1 := previousSample.Price
			price2 := sample.Price
			if !includePrices(price1, price2) {
				continue
			}
			returns := getRateOfChange(price2, price1)
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
	entries := sortMapByKey(weekdayMap, cmp.Compare)
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
		15, 30,
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
	fillPriceRangeBins(negRisk, samplingHour, offset, priceRangeBins, historyData)
	fmt.Printf("Outcomes by rice range (negRisk = %t, samplingHour = %dh, offset = %d):\n", negRisk, samplingHour, offset)
	for _, priceRange := range priceRangeBins {
		meanDelta := stat.Mean(priceRange.deltas, nil)
		fmt.Printf("\t%.2f - %.2f: %.3f\n", priceRange.rangeMin, priceRange.rangeMax, meanDelta)
	}
	fmt.Printf("\n")
}

func analyzeCategoryPriceRanges(historyData []PriceHistoryBSON) {
	negRisk := true
	samplingHour := 15
	offset := 15
	limits := [][]float64{
		{0.7, 0.9},
	}
	for _, rangeMinMax := range limits {
		rangeMin := rangeMinMax[0]
		rangeMax := rangeMinMax[1]
		analyzeCategoryPriceRange(negRisk, samplingHour, offset, rangeMin, rangeMax, historyData)
	}
}

func analyzeCategoryPriceRange(
	negRisk bool,
	samplingHour int,
	offset int,
	rangeMin float64,
	rangeMax float64,
	historyData []PriceHistoryBSON,
) {
	tagHistories := map[string][]PriceHistoryBSON{}
	for _, history := range historyData {
		if !includeHistory(negRisk, history) {
			continue
		}
		for _, tag := range history.Tags {
			tagHistories[tag] = append(tagHistories[tag], history)
		}
	}
	tagDeltas := []tagDeltaData{}
	for tag, tagHistoryData := range tagHistories {
		priceRangeBins := []priceRangeReturns{
			{
				rangeMin: rangeMin,
				rangeMax: rangeMax,
			},
		}
		fillPriceRangeBins(negRisk, samplingHour, offset, priceRangeBins, tagHistoryData)
		bin := priceRangeBins[0]
		deltas := bin.deltas
		samples := len(deltas)
		if samples == 0 {
			continue
		}
		meanDelta := stat.Mean(deltas, nil)
		meanYes := stat.Mean(bin.yesReturns, nil)
		meanNo := stat.Mean(bin.noReturns, nil)
		tagDelta := tagDeltaData{
			tag: tag,
			delta: meanDelta,
			yes: meanYes,
			no: meanNo,
			samples: samples,
		}
		tagDeltas = append(tagDeltas, tagDelta)
	}
	slices.SortFunc(tagDeltas, func (a, b tagDeltaData) int {
		return cmp.Compare(a.delta, b.delta)
	})
	format := "Outcomes by category (negRisk = %t, samplingHour = %d, offset = %d, rangeMin = %.2f, rangeMax = %.2f)\n"
	fmt.Printf(format, negRisk, samplingHour, offset, rangeMin, rangeMax)
	row := 1
	for _, tagDelta := range tagDeltas {
		if tagDelta.samples < tagSamplesMin {
			continue
		}
		format := "\t%d. %s: delta = %.3f, yes = %.2f%%, no = %.2f%%, samples = %d\n"
		fmt.Printf(format, row, tagDelta.tag, tagDelta.delta, tagDelta.yes * percent, tagDelta.no * percent, tagDelta.samples)
		row += 1
	}
}

func fillPriceRangeBins(
	negRisk bool,
	samplingHour int,
	offset int,
	priceRangeBins []priceRangeReturns,
	historyData []PriceHistoryBSON,
) {
	for _, history := range historyData {
		if !includeHistory(negRisk, history) {
			continue
		}
		samples := getSamplingHours(samplingHour, history)
		for i := range priceRangeBins {
			priceRange := &priceRangeBins[i]
			priceRange.hit = false
		}
		for i := range samples {
			if i < priceRangeBinsDaysMin {
				continue
			}
			price1 := samples[i].Price
			for j := range priceRangeBins {
				priceRange := &priceRangeBins[j]
				if !priceRange.hit && price1 >= priceRange.rangeMin && price1 < priceRange.rangeMax {
					adjustedOffset := i + offset
					if adjustedOffset >= len(samples) {
						adjustedOffset = len(samples) - 1
					}
					price2 := samples[adjustedOffset].Price
					delta := price2 - price1
					yesReturns := getRateOfChange(price2 - spread, price1)
					noPrice1 := 1.0 - price1
					noPrice2 := 1.0 - price2
					noReturns := getRateOfChange(noPrice2 - spread, noPrice1)
					priceRange.deltas = append(priceRange.deltas, delta)
					priceRange.yesReturns = append(priceRange.yesReturns, yesReturns)
					priceRange.noReturns = append(priceRange.noReturns, noReturns)
					priceRange.hit = true
				}
			}
		}
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

func includePrices(price1, price2 float64) bool {
	match1 := price1 >= seasonalityPriceMin && price1 <= seasonalityPriceMax
	match2 := price2 >= seasonalityPriceMin && price2 <= seasonalityPriceMax
	return match1 && match2
}

func getSamplingHours(samplingHour int, history PriceHistoryBSON) []PriceHistorySampleBSON {
	samples := []PriceHistorySampleBSON{}
	previousDate := time.Time{}
	hasPreviousSample := false
	for _, sample := range history.History {
		timestamp := sample.Timestamp
		date := commons.GetDate(timestamp)
		if date.Year() < dataMinYear {
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

func includeHistory(negRisk bool, history PriceHistoryBSON) bool {
	include := history.Closed
	include = include && history.StartDate.Year() >= dataMinYear
	include = include && history.NegRisk == negRisk
	include = include && len(history.History) >= minHistorySize
	return include
}