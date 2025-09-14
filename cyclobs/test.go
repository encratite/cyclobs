package cyclobs

import (
	"fmt"
	"time"
)

func Backtest() {
	loadConfiguration()
	// backtestDecaySingle()
	// backtestDecayHeatmaps()
	// backtestThresholdSingle()
	backtestJumpSingle()
}

func backtestDecaySingle() {
	start := getDateFromString("2024-02-01")
	end := getDateFromString("2025-09-15")
	tags := []string{
		"business",
		"world",
		"elections",
		"trump",
		/*
		"politics",
		"geopolitics",
		"world",
		"trump",
		"trump-presidency",
		"finance",
		"business",
		*/
	}
	const (
		positionSize = 10.0
		holdingTime = 30 * 24
		priceRangeCheck = true
		triggerPriceMin = 0.5
		triggerPriceMax = 0.9
	)
	strategy := decayStrategy{
		tags: tags,
		triggerPriceMin: triggerPriceMin,
		triggerPriceMax: triggerPriceMax,
		positionSize: positionSize,
		holdingTime: holdingTime,
		priceRangeCheck: priceRangeCheck,
	}
	historyMap, dailyData, prices := loadBacktestData()
	result := runBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func backtestDecayHeatmaps() {
	start := getDateFromString("2024-01-01")
	end := getDateFromString("2025-09-15")
	tags := []string{
		"politics",
		"geopolitics",
		"world",
		"elections",
		"trump",
		"trump-presidency",
		"finance",
		"business",
		"tech",
		/*
		"ai",
		"hide-from-new",
		"recurring",
		"weekly",
		"crypto",
		"crypto-prices",
		"bitcoin",
		"ethereum",
		"solana",
		*/
	}
	const (
		positionSize = 20.0
		holdingTime = 7 * 24
		priceRangeCheck = false
		steps = 10
		factor = 1.0 / float64(steps)
	)
	strategies := []decayStrategy{}
	for _, tag := range tags {
		for i := range steps {
			strategyTags := []string{
				tag,
			}
			triggerPriceMin := factor * float64(i)
			triggerPriceMax := factor * float64(i + 1)
			strategy := decayStrategy{
				tags: strategyTags,
				triggerPriceMin: triggerPriceMin,
				triggerPriceMax: triggerPriceMax,
				positionSize: positionSize,
				holdingTime: holdingTime,
				priceRangeCheck: priceRangeCheck,
			}
			strategies = append(strategies, strategy)
		}
	}
	historyMap, dailyData, prices := loadBacktestData()
	backtestStart := time.Now()
	results := parallelMap(strategies, func (strategy decayStrategy) StrategyResult {
		result := runBacktest(&strategy, start, end, historyMap, dailyData, prices)
		strategyResult := StrategyResult{
			Tag: strategy.tags[0],
			Parameter: fmt.Sprintf("%.1f - %.1f", strategy.triggerPriceMin, strategy.triggerPriceMax),
			SharpeRatio: result.sharpeRatio,
		}
		return strategyResult
	})
	backtestEnd := time.Now()
	backtestDuration := backtestEnd.Sub(backtestStart)
	fmt.Printf("Backtest finished after %.1f s\n", backtestDuration.Seconds())
	plotData("heatmap", results)
}

func backtestThresholdSingle() {
	start := getDateFromString("2024-01-01")
	end := getDateFromString("2025-09-15")
	tags := []string{
		"politics",
		// "geopolitics",
		// "world",
		// "trump",
		// "trump-presidency",
		// "finance",
		// "business",
	}
	const (
		positionSize = 25.0
		threshold = 0.95
		greaterThan = true
		side = sideYes
	)
	strategy := thresholdStrategy{
		tags: tags,
		threshold: threshold,
		greaterThan: greaterThan,
		positionSize: positionSize,
		side: side,
	}
	historyMap, dailyData, prices := loadBacktestData()
	result := runBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func backtestJumpSingle() {
	start := getDateFromString("2024-01-01")
	end := getDateFromString("2025-09-15")
	tags := []string{
		/*
		"politics",
		"geopolitics",
		"world",
		"trump",
		"trump-presidency",
		*/
	}
	const (
		threshold1 = 0.3
		threshold2 = 0.5
		positionSize = 50.0
		holdingTime = 1 * 24
	)
	strategy := jumpStrategy{
		tags: tags,
		threshold1: threshold1,
		threshold2: threshold2,
		positionSize: positionSize,
		holdingTime: holdingTime,
		previousPrices: map[string]priceSample{},
	}
	historyMap, dailyData, prices := loadBacktestData()
	result := runBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}