package main

import (
	"cmp"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/encratite/commons"
	"github.com/gammazero/deque"
	"gonum.org/v1/gonum/stat"
)

const (
	backtestMinVolume = 100000.0
	backtestInitialCash = 2000.0
	backtestPriceMin = 0.001
	backtestPriceMax = 0.999
	backtestDebugPositions = false
	backtestMaxPriceOffset = 10 * 24
	backtestNegRisk = false
	backtestPrintTags = false
	backtestPrintHours = true
	backtestPrintWeekdays = true
	backtestPrintPrice = true
	backtestPrintRecentTrades = true
	backtestRecentTradesLimit = 10
	riskFreeRate = 0.045
	monthsPerYear = 12
	sharpeRatioMinSamples = 5
	spreadFactor = 1.5
)

type backtestPositionSide int

const (
	sideNo backtestPositionSide = iota
	sideYes
)

type backtestStrategy interface {
	next(backtest *backtestData)
}

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
	equityCurve []EquityCurveSample
	historyMap map[string]*PriceHistoryBSON
	dailyData map[time.Time]backtestDailyData
	prices map[backtestPriceKey]float64
	tagPerformance map[string]performanceData[string]
	hourPerformance map[int]performanceData[int]
	weekdayPerformance map[int]performanceData[int]
	pricePerformance map[int]performanceData[int]
	recentTrades deque.Deque[backtestTrade]
}

type backtestPosition struct {
	slug string
	timestamp time.Time
	side backtestPositionSide
	price float64
	size float64
}

type EquityCurveSample struct {
	Timestamp time.Time `json:"date"`
	Cash float64 `json:"cash"`
}

type backtestResult struct {
	start time.Time
	end time.Time
	cash float64
	totalReturn float64
	maxDrawdown float64
	sharpeRatio float64
	trades int
	equityCurve []EquityCurveSample
	tagPerformance []performanceData[string]
	hourPerformance []performanceData[int]
	weekdayPerformance []performanceData[int]
	pricePerformance []performanceData[int]
	recentTrades deque.Deque[backtestTrade]
}

type performanceData[K any] struct {
	key K
	profit float64
	trades int
	returns []float64
}

type backtestTrade struct {
	timestamp time.Time
	slug string
	profit float64
}

func loadBacktestData() (map[string]*PriceHistoryBSON, map[time.Time]backtestDailyData, map[backtestPriceKey]float64) {
	database := newDatabaseClient()
	defer database.close()
	negRisk := backtestNegRisk
	minVolume := backtestMinVolume
	historyData := database.getPriceHistoryData(nil, &negRisk, &minVolume, nil)
	historyMap := map[string]*PriceHistoryBSON{}
	dailyData := map[time.Time]backtestDailyData{}
	prices := map[backtestPriceKey]float64{}
	for i := range historyData {
		history := &historyData[i]
		historyMap[history.Slug] = history
		for _, price := range history.History {
			date := commons.GetDate(price.Timestamp)
			data, exists := dailyData[date]
			if !exists {
				data = backtestDailyData{
					historyData: map[string]*PriceHistoryBSON{},
				}
			}
			data.historyData[history.Slug] = history
			dailyData[date] = data
			hourTimestamp := commons.GetHourTimestamp(price.Timestamp)
			priceKey := backtestPriceKey{
				slug: history.Slug,
				timestamp: hourTimestamp,
			}
			prices[priceKey] = price.Price
		}
	}
	return historyMap, dailyData, prices
}

func executeBacktest(
	strategy backtestStrategy,
	start time.Time,
	end time.Time,
	historyMap map[string]*PriceHistoryBSON,
	dailyData map[time.Time]backtestDailyData,
	prices map[backtestPriceKey]float64,
) backtestResult {
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
		tagPerformance: map[string]performanceData[string]{},
		hourPerformance: map[int]performanceData[int]{},
		weekdayPerformance: map[int]performanceData[int]{},
		pricePerformance: map[int]performanceData[int]{},
		recentTrades: deque.Deque[backtestTrade]{},
	}
	sample := EquityCurveSample{
		Timestamp: commons.GetDate(start),
		Cash: backtestInitialCash,
	}
	backtest.equityCurve = []EquityCurveSample{
		sample,
	}
	for backtest.now.Before(end) {
		strategy.next(&backtest)
		backtest.resolveMarkets()
		backtest.updateStats()
		backtest.now = backtest.now.Add(time.Hour)
	}
	backtest.closeAllPositions()
	backtest.addEquityCurveSample(end, backtest.cash)
	totalReturn := getRateOfChange(backtest.cash, backtestInitialCash)
	sharpeRatio := backtest.getSharpeRatio()
	tagPerformance := sortMapByValue(backtest.tagPerformance, func (a, b performanceData[string]) int {
		return cmp.Compare(b.trades, a.trades)
	})
	hourPerformance := sortMapByValue(backtest.hourPerformance, func (a, b performanceData[int]) int {
		return cmp.Compare(a.key, b.key)
	})
	weekdayPerformance := sortMapByValue(backtest.weekdayPerformance, func (a, b performanceData[int]) int {
		return cmp.Compare(a.key, b.key)
	})
	pricePerformance := sortMapByValue(backtest.pricePerformance, func (a, b performanceData[int]) int {
		return cmp.Compare(a.key, b.key)
	})
	result := backtestResult{
		start: start,
		end: end,
		cash: backtest.cash,
		totalReturn: totalReturn,
		maxDrawdown: backtest.maxDrawdown,
		sharpeRatio: sharpeRatio,
		trades: backtest.trades,
		equityCurve: backtest.equityCurve,
		tagPerformance: tagPerformance,
		hourPerformance: hourPerformance,
		weekdayPerformance: weekdayPerformance,
		pricePerformance: pricePerformance,
		recentTrades: backtest.recentTrades,
	}
	return result
}

func (b *backtestData) getMarkets(tags []string) []*PriceHistoryBSON {
	date := commons.GetDate(b.now)
	dailyData, exists := b.dailyData[date]
	if !exists {
		return nil
	}
	historyData := []*PriceHistoryBSON{}
	for _, history := range dailyData.historyData {
		if len(tags) > 0 {
			for _, tag := range tags {
				if commons.Contains(history.Tags, tag) {
					historyData = append(historyData, history)
					break
				}
			}
		} else {
			historyData = append(historyData, history)
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
				fmt.Printf("%d. %s %.3f\n", i + 1, commons.GetTimeString(sample.Timestamp), sample.Price)
			}
		}
	}
	log.Fatalf("Failed to determine the price of %s at %s", slug, commons.GetTimeString(b.now))
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
		fmt.Printf(format, commons.GetTimeString(b.now), getSideString(side), slug, ask, commons.FormatMoney(b.cash))
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
			profit := position.size * (bid - position.price)
			if backtestDebugPositions {
				format := "%s Closed \"%s\" position on %s at %.3f (%s)\n"
				fmt.Printf(format, commons.GetTimeString(b.now), getSideString(position.side), slug, bid, commons.FormatMoney(b.cash))
			}
			b.updatePerformanceStats(slug, profit, position)
			if backtestPrintRecentTrades {
				trade := backtestTrade{
					timestamp: position.timestamp,
					slug: slug,
					profit: profit,
				}
				b.recentTrades.PushBack(trade)
				for b.recentTrades.Len() > backtestRecentTradesLimit {
					b.recentTrades.PopFront()
				}
			}
			hit = true
		} else {
			newPositions = append(newPositions, position)
		}
	}
	b.positions = newPositions
	return hit
}

func (b *backtestData) updatePerformanceStats(slug string, profit float64, position backtestPosition) {
	b.trades++
	history, exists := b.historyMap[slug]
	if !exists {
		return
	}
	for _, tag := range history.Tags {
		addPerformance(tag, profit, position, b.tagPerformance)
	}
	hour := position.timestamp.Hour() / 4
	addPerformance(hour, profit, position, b.hourPerformance)
	weekday := int(position.timestamp.Weekday())
	addPerformance(weekday, profit, position, b.weekdayPerformance)
	priceBin := int(10 * position.price)
	addPerformance(priceBin, profit, position, b.pricePerformance)
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
		timestamp := commons.GetHourTimestamp(last.Timestamp)
		if b.now.Equal(timestamp) || b.now.After(timestamp) {
			if market.Closed && market.Outcome != nil {
				newPositions := []backtestPosition{}
				for _, position := range b.positions {
					if position.slug == market.Slug {
						yes := position.side == sideYes
						payout := 0.0
						if *market.Outcome == yes {
							payout = 1.0
							b.cash += position.size * payout
						}
						profit := position.size * (payout - position.price)
						b.updatePerformanceStats(position.slug, profit, position)
					} else {
						newPositions = append(newPositions, position)
					}
				}
				b.positions = newPositions
			} else {
				b.closePositions(market.Slug)
			}
		}
	}
}

func (b *backtestData) updateStats() {
	netWorth := b.getNetWorth()
	b.maxCash = max(b.maxCash, netWorth)
	drawdown := 1.0 - netWorth / b.maxCash
	b.maxDrawdown = max(b.maxDrawdown, drawdown)
	b.addEquityCurveSample(b.now, netWorth)
}

func (b *backtestData) getSharpeRatio() float64 {
	returns := []float64{}
	previousSample := b.equityCurve[0]
	for _, sample := range b.equityCurve[1:] {
		if sample.Timestamp.Month() != previousSample.Timestamp.Month() {
			monthlyReturns := getRateOfChange(sample.Cash, previousSample.Cash)
			returns = append(returns, monthlyReturns)
			previousSample = sample
		}
	}
	if len(returns) < sharpeRatioMinSamples {
		return 0.0
	}
	annualRate := riskFreeRate
	monthlyRate := math.Pow(1.0 + annualRate, 1.0 / monthsPerYear) - 1.0
	sharpeRatio := (stat.Mean(returns, nil) - monthlyRate) / stat.StdDev(returns, nil)
	annualizedSharpe := math.Sqrt(monthsPerYear) * sharpeRatio
	if math.IsInf(annualizedSharpe, 0) {
		return 0.0
	}
	return annualizedSharpe
}

func (b *backtestData) addEquityCurveSample(date time.Time, cash float64) {
	sample := EquityCurveSample{
		Timestamp: date,
		Cash: cash,
	}
	b.equityCurve = append(b.equityCurve, sample)
}

func (r *backtestResult) print() {
	fmt.Printf("\tStart: %s\n", commons.GetDateString(r.start))
	fmt.Printf("\tEnd: %s\n", commons.GetDateString(r.end))
	fmt.Printf("\tCash: %s\n", commons.FormatMoney(r.cash))
	fmt.Printf("\tTotal return: %+.1f%%\n", percent * r.totalReturn)
	fmt.Printf("\tMax drawdown: %.2f%%\n", percent * r.maxDrawdown)
	fmt.Printf("\tSharpe ratio: %.2f\n", r.sharpeRatio)
	fmt.Printf("\tTrades: %d\n", r.trades)
	if backtestPrintTags {
		fmt.Printf("\n\tProfit by tag:\n")
		for i, performance := range r.tagPerformance {
			if i >= 25 {
				break
			}
			fmt.Printf("\t\t%d. %s: %s (%d trades)\n", i + 1, performance.key, commons.FormatMoney(performance.profit), performance.trades)
		}
	}
	if backtestPrintHours {
		fmt.Printf("\n\tProfit by hour:\n")
		for _, performance := range r.hourPerformance {
			hour1 := 4 * performance.key
			hour2 := 4 * (performance.key + 1)
			profit, riskAdjusted := performance.getStats()
			format := "\t\t%02d:00 - %02d:00: %.2f RAR, $%.2f/trade, %d trades\n"
			fmt.Printf(format, hour1, hour2, riskAdjusted, profit, performance.trades)
		}
	}
	if backtestPrintWeekdays {
		fmt.Printf("\n\tProfit by weekday:\n")
		for _, performance := range r.weekdayPerformance {
			weekday := time.Weekday(performance.key)
			profit, riskAdjusted := performance.getStats()
			format := "\t\t%s: %.2f RAR, $%.2f/trade, %d trades\n"
			fmt.Printf(format, weekday, riskAdjusted, profit, performance.trades)
		}
	}
	if backtestPrintPrice {
		fmt.Printf("\n\tProfit by initial price:\n")
		for _, performance := range r.pricePerformance {
			price1 := float64(performance.key) / 10.0
			price2 := float64(performance.key + 1) / 10.0
			profit, riskAdjusted := performance.getStats()
			fmt.Printf("\t\t%.1f - %.1f: %.2f RAR, $%.2f/trade, %d trades\n", price1, price2, riskAdjusted, profit, performance.trades)
		}
	}
	if backtestPrintRecentTrades {
		fmt.Printf("\n\tRecent trades:\n")
		for trade := range r.recentTrades.Iter() {
			fmt.Printf("\t\t%s %s: %s\n", commons.GetTimeString(trade.timestamp), trade.slug, commons.FormatMoney(trade.profit))
		}
	}
}

func (p* performanceData[K]) getStats() (float64, float64) {
	profit := p.profit / float64(p.trades)
	riskAdjusted := stat.Mean(p.returns, nil) / stat.StdDev(p.returns, nil)
	return profit, riskAdjusted
}

func addPerformance[K comparable](key K, profit float64, position backtestPosition, performanceMap map[K]performanceData[K]) {
	returns := profit / (position.price * position.size)
	performance, exists := performanceMap[key]
	if !exists {
		performance = performanceData[K]{
			key: key,
			profit: 0.0,
			trades: 0,
			returns: []float64{},
		}
	}
	performance.profit += profit
	performance.trades++
	performance.returns = append(performance.returns, returns)
	performanceMap[key] = performance
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
	price = max(price, 0.0)
	price = min(price, 1.0)
	spread := 0.01
	if price <= 0.06 || price >= 0.94 {
		spread = 0.001
	}
	spread *= spreadFactor
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

func getDailyEquityCurve(equityCurve []EquityCurveSample) []EquityCurveSample {
	output := []EquityCurveSample{}
	for _, sample := range equityCurve {
		if len(output) > 0 {
			previousSample := output[len(output) - 1]
			date := commons.GetDate(sample.Timestamp)
			previousDate := commons.GetDate(previousSample.Timestamp)
			if !date.Equal(previousDate) {
				output = append(output, sample)
			}
		} else {
			output = append(output, sample)
		}
	}
	return output
}