package cyclobs

import (
	"cmp"
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
	backtestMaxPriceOffset = 10 * 24
	backtestNegRisk = false
	backtestPrintTags = false
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
	tagPerformance map[string]tagPerformanceData
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
	tagPerformance []tagPerformanceData
}

type tagPerformanceData struct {
	tag string
	profit float64
	trades int
}

func loadBacktestData() (map[string]*PriceHistoryBSON, map[time.Time]backtestDailyData, map[backtestPriceKey]float64) {
	database := newDatabaseClient()
	defer database.close()
	negRisk := backtestNegRisk
	minVolume := backtestMinVolume
	historyData := database.getPriceHistoryData(nil, &negRisk, &minVolume)
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
		tagPerformance: map[string]tagPerformanceData{},
	}
	sample := EquityCurveSample{
		Timestamp: getDate(start),
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
	tagPerformance := sortMapByValue(backtest.tagPerformance, func (a, b tagPerformanceData) int {
		return cmp.Compare(b.trades, a.trades)
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
	}
	return result
}

func (b *backtestData) getMarkets(tags []string) []*PriceHistoryBSON {
	date := getDate(b.now)
	dailyData, exists := b.dailyData[date]
	if !exists {
		return nil
	}
	historyData := []*PriceHistoryBSON{}
	for _, history := range dailyData.historyData {
		if len(tags) > 0 {
			for _, tag := range tags {
				if contains(history.Tags, tag) {
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
			profit := position.size * (bid - position.price)
			if backtestDebugPositions {
				format := "%s Closed \"%s\" position on %s at %.3f (%s)\n"
				fmt.Printf(format, getTimeString(b.now), getSideString(position.side), slug, bid, formatMoney(b.cash))
			}
			b.trades++
			hit = true
			history, exists := b.historyMap[slug]
			if !exists {
				continue
			}
			for _, tag := range history.Tags {
				tagPerformance, exists := b.tagPerformance[tag]
				if !exists {
					tagPerformance = tagPerformanceData{
						tag: tag,
						profit: 0.0,
						trades: 0,
					}
				}
				tagPerformance.profit += profit
				tagPerformance.trades++
				b.tagPerformance[tag] = tagPerformance
			}
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
			if market.Closed && market.Outcome != nil {
				newPositions := []backtestPosition{}
				for _, position := range b.positions {
					if position.slug == market.Slug {
						yes := position.side == sideYes
						if *market.Outcome == yes {
							b.cash += position.size
						}
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
	fmt.Printf("\tStart: %s\n", getDateString(r.start))
	fmt.Printf("\tEnd: %s\n", getDateString(r.end))
	fmt.Printf("\tCash: %s\n", formatMoney(r.cash))
	fmt.Printf("\tTotal return: %+.1f%%\n", percent * r.totalReturn)
	fmt.Printf("\tMax drawdown: %.2f%%\n", percent * r.maxDrawdown)
	fmt.Printf("\tSharpe ratio: %.2f\n", r.sharpeRatio)
	fmt.Printf("\tTrades: %d\n\n", r.trades)
	if backtestPrintTags {
		fmt.Printf("\tProfit by tag:\n")
		for i, performance := range r.tagPerformance {
			if i >= 25 {
				break
			}
			fmt.Printf("\t\t%d. %s: %s (%d trades)\n", i + 1, performance.tag, formatMoney(performance.profit), performance.trades)
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
			date := getDate(sample.Timestamp)
			previousDate := getDate(previousSample.Timestamp)
			if !date.Equal(previousDate) {
				output = append(output, sample)
			}
		} else {
			output = append(output, sample)
		}
	}
	return output
}