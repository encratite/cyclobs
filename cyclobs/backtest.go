package cyclobs

import (
	"fmt"
	"log"
	"math"
	"time"

	"gonum.org/v1/gonum/stat"
)

const (
	backtestMinVolume = 100000.0
	backtestInitialCash = 2000.0
	backtestPriceMin = 0.001
	backtestPriceMax = 0.999
	backtestDebugPositions = false
	backtestMaxPriceOffset = 48
	riskFreeRate = 0.045
	monthsPerYear = 12
)

type backtestPositionSide int

const (
	sideNo backtestPositionSide = iota
	sideYes
)

type backtestDailyData struct {
	historyData map[string]*PriceHistoryBSON
}

type backtestPriceKey struct {
	slug string
	timestamp time.Time
}

type backtestData struct {
	cash float64
	maxCash float64
	maxDrawdown float64
	positions []backtestPosition
	now time.Time
	trades int
	equityCurve []equityCurveSample
	historyMap map[string]*PriceHistoryBSON
	dailyData map[time.Time]backtestDailyData
	prices map[backtestPriceKey]float64
}

type backtestPosition struct {
	slug string
	timestamp time.Time
	side backtestPositionSide
	price float64
	size float64
}

type equityCurveSample struct {
	date time.Time
	cash float64
}

func Backtest() {
	loadConfiguration()
	historyMap, dailyData, prices := loadBacktestData()
	start := getDateFromString("2024-01-01")
	end := getDateFromString("2025-10-01")
	runBacktest(start, end, historyMap, dailyData, prices)
}

func loadBacktestData() (map[string]*PriceHistoryBSON, map[time.Time]backtestDailyData, map[backtestPriceKey]float64) {
	database := newDatabaseClient()
	defer database.close()
	closed := true
	negRisk := false
	minVolume := backtestMinVolume
	historyData := database.getPriceHistoryData(&closed, &negRisk, &minVolume)
	historyMap := map[string]*PriceHistoryBSON{}
	dailyData := map[time.Time]backtestDailyData{}
	prices := map[backtestPriceKey]float64{}
	for i := range historyData {
		history := &historyData[i]
		historyMap[history.Slug] = history
		for _, price := range history.History {
			date := getDate(price.Timestamp)
			data, exists := dailyData[date]
			if !exists {
				data = backtestDailyData{
					historyData: map[string]*PriceHistoryBSON{},
				}
			}
			data.historyData[history.Slug] = history
			dailyData[date] = data
			hourTimestamp := getHourTimestamp(price.Timestamp)
			priceKey := backtestPriceKey{
				slug: history.Slug,
				timestamp: hourTimestamp,
			}
			prices[priceKey] = price.Price
		}
	}
	return historyMap, dailyData, prices
}

func runBacktest(
	start time.Time,
	end time.Time,
	historyMap map[string]*PriceHistoryBSON,
	dailyData map[time.Time]backtestDailyData,
	prices map[backtestPriceKey]float64,
) {
	backtest := backtestData{
		cash: backtestInitialCash,
		maxCash: backtestInitialCash,
		maxDrawdown: 0.0,
		positions: []backtestPosition{},
		now: start,
		trades: 0,
		historyMap: historyMap,
		dailyData: dailyData,
		prices: prices,
	}
	sample := equityCurveSample{
		date: getDate(start),
		cash: backtestInitialCash,
	}
	backtest.equityCurve = []equityCurveSample{
		sample,
	}
	backtestStart := time.Now()
	for backtest.now.Before(end) {
		executeStrategy(&backtest)
		backtest.resolveMarkets()
		backtest.updateStats()
		backtest.now = backtest.now.Add(time.Hour)
	}
	backtest.addEquityCurveSample(end, backtest.cash)
	backtestEnd := time.Now()
	backtestDuration := backtestEnd.Sub(backtestStart)
	backtest.closeAllPositions()
	totalReturn := getRateOfChange(backtest.cash, backtestInitialCash)
	sharpeRatio := backtest.getSharpeRatio()
	fmt.Printf("\nBacktest concluded after %.1f s\n\n", backtestDuration.Seconds())
	fmt.Printf("\tStart: %s\n", getDateString(start))
	fmt.Printf("\tEnd: %s\n", getDateString(end))
	fmt.Printf("\tCash: %s\n", formatMoney(backtest.cash))
	fmt.Printf("\tTotal return: %+.1f%%\n", percent * totalReturn)
	fmt.Printf("\tMax drawdown: %.2f%%\n", percent * backtest.maxDrawdown)
	fmt.Printf("\tSharpe ratio: %.2f\n", sharpeRatio)
	fmt.Printf("\tTrades: %d\n\n", backtest.trades)
}

func (b *backtestData) getMarkets(tags []string) []*PriceHistoryBSON {
	date := getDate(b.now)
	dailyData, exists := b.dailyData[date]
	if !exists {
		return nil
	}
	historyData := []*PriceHistoryBSON{}
	for _, history := range dailyData.historyData {
		for _, tag := range tags {
			if contains(history.Tags, tag) {
				historyData = append(historyData, history)
				break
			}
		}
	}
	return historyData
}

func (b *backtestData) getPriceErr(slug string) (float64, bool) {
	for i := range backtestMaxPriceOffset {
		duration := time.Duration(- i) * time.Hour
		timestamp := b.now.Add(duration)
		priceKey := backtestPriceKey{
			slug: slug,
			timestamp: timestamp,
		}
		price, exists := b.prices[priceKey]
		if !exists {
			continue
		}
		return price, true
	}
	return 0.0, false
}

func (b *backtestData) getPrice(slug string) float64 {
	price, exists := b.getPriceErr(slug)
	if exists {
		return price
	}
	history, exists := b.historyMap[slug]
	if exists {
		const offset = 5
		for i, sample := range history.History {
			if i < offset || i >= len(history.History) - offset {
				fmt.Printf("%d. %s %.3f\n", i + 1, getTimeString(sample.Timestamp), sample.Price)
			}
		}
	}
	log.Fatalf("Failed to determine the price of %s at %s", slug, getTimeString(b.now))
	return 0.0
}

func (b *backtestData) openPosition(slug string, side backtestPositionSide, size float64) bool {
	price, exists := b.getPriceErr(slug)
	if !exists {
		return false
	}
	_, ask := getBidAsk(price, side)
	cost := size * ask
	if cost > b.cash {
		return false
	}
	position := backtestPosition{
		slug: slug,
		timestamp: b.now,
		side: side,
		price: ask,
		size: size,
	}
	b.positions = append(b.positions, position)
	b.cash -= cost
	if backtestDebugPositions {
		format := "%s Opened \"%s\" position on %s at %.3f (%s)\n"
		fmt.Printf(format, getTimeString(b.now), getSideString(side), slug, ask, formatMoney(b.cash))
	}
	return true
}

func (b *backtestData) closeAllPositions() {
	for _, position := range b.positions {
		_ = b.closePositions(position.slug)
	}
}

func (b *backtestData) closePositions(slug string) bool {
	hit := false
	newPositions := []backtestPosition{}
	for _, position := range b.positions {
		if position.slug == slug {
			price := b.getPrice(slug)
			bid, _ := getBidAsk(price, position.side)
			b.cash += position.size * bid
			if backtestDebugPositions {
				format := "%s Closed \"%s\" position on %s at %.3f (%s)\n"
				fmt.Printf(format, getTimeString(b.now), getSideString(position.side), slug, bid, formatMoney(b.cash))
			}
			b.trades++
			hit = true
		} else {
			newPositions = append(newPositions, position)
		}
	}
	b.positions = newPositions
	return hit
}

func (b *backtestData) getNetWorth() float64 {
	netWorth := b.cash
	for _, position := range b.positions {
		price := b.getPrice(position.slug)
		bid, _ := getBidAsk(price, position.side)
		netWorth += position.size * bid
	}
	return netWorth
}

func (b *backtestData) resolveMarkets() {
	for _, position := range b.positions {
		market, exists := b.historyMap[position.slug]
		if !exists {
			log.Fatalf("Unable to find market for position %s", position.slug)
		}
		last := market.History[len(market.History) - 1]
		timestamp := getHourTimestamp(last.Timestamp)
		if b.now.Equal(timestamp) || b.now.After(timestamp) {
			b.closePositions(market.Slug)
		}
	}
}

func (b *backtestData) updateStats() {
	netWorth := b.getNetWorth()
	b.maxCash = max(b.maxCash, netWorth)
	drawdown := 1.0 - b.cash / b.maxCash
	b.maxDrawdown = max(b.maxDrawdown, drawdown)
	date := getDate(b.now)
	b.addEquityCurveSample(date, netWorth)
}

func (b *backtestData) getSharpeRatio() float64 {
	returns := []float64{}
	previousSample := b.equityCurve[0]
	for _, sample := range b.equityCurve[1:] {
		if sample.date.Month() != previousSample.date.Month() {
			monthlyReturns := getRateOfChange(sample.cash, previousSample.cash)
			returns = append(returns, monthlyReturns)
			previousSample = sample
		}
	}
	annualRate := riskFreeRate
	monthlyRate := math.Pow(1.0 + annualRate, 1.0 / monthsPerYear) - 1.0
	sharpeRatio := (stat.Mean(returns, nil) - monthlyRate) / stat.StdDev(returns, nil)
	annualizedSharpe := math.Sqrt(monthsPerYear) * sharpeRatio
	return annualizedSharpe
}

func (b *backtestData) addEquityCurveSample(date time.Time, cash float64) {
	sample := equityCurveSample{
		date: date,
		cash: cash,
	}
	b.equityCurve = append(b.equityCurve, sample)
}

func executeStrategy(backtest *backtestData) {
	tags := []string{
		"trump",
		"trump-presidency",
		"hide-from-new",
		"weekly",
		"crypto",
		"crypto-prices",
		"bitcoin",
		"ethereum",
	}
	const (
		triggerPriceMin = 0.8
		triggerPriceMax = 0.9
		positionSize = 25.0
		holdingTime = 15 * 24
		priceRangeCheck = false
	)
	markets := backtest.getMarkets(tags)
	for _, market := range markets {
		price, exists := backtest.getPriceErr(market.Slug)
		if !exists {
			continue
		}
		if price >= triggerPriceMin && price < triggerPriceMax {
			exists := containsFunc(backtest.positions, func (p backtestPosition) bool {
				return p.slug == market.Slug
			})
			if exists {
				continue
			}
			size := positionSize / price
			_ = backtest.openPosition(market.Slug, sideNo, size)
		}
		for _, position := range backtest.positions {
			expired := backtest.now.Sub(position.timestamp) >= time.Duration(holdingTime) * time.Hour
			if priceRangeCheck {
				price := backtest.getPrice(position.slug)
				priceInRange := price >= triggerPriceMin && price < triggerPriceMax
				if expired || !priceInRange {
					backtest.closePositions(position.slug)
				}
			} else {
				if expired {
					backtest.closePositions(position.slug)
				}
			}
		}
	}
}

func normalizePrice(price float64) float64 {
	price = max(price, backtestPriceMin)
	price = min(price, backtestPriceMax)
	return price
}

func convertPrice(price float64, side backtestPositionSide) float64 {
	if side == sideNo {
		price = 1.0 - price
	}
	return price
}

func getBidAsk(price float64, side backtestPositionSide) (float64, float64) {
	if price < 0.0 || price > 1.0 {
		log.Fatalf("Encountered an invalid price: %.3f", price)
	}
	spread := 0.01
	if price <= 0.06 || price >= 0.94 {
		spread = 0.001
	}
	price = convertPrice(price, side)
	bid := normalizePrice(price - spread)
	ask := normalizePrice(price + spread)
	return bid, ask
}

func getSideString(side backtestPositionSide) string {
	switch side {
	case sideYes:
		return "yes"
	case sideNo:
		return "no"
	}
	log.Fatalf("Unknown side in getSideString: %d", side)
	return "?"
}