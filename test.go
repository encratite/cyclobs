package main

import (
	"fmt"
	"time"

	"github.com/encratite/commons"
)

func runBacktest() {
	loadConfiguration()
	// backtestDecaySingle()
	// backtestDecayHeatmaps()
	// backtestThresholdSingle()
	backtestJump()
	// backtestMention()
}

func backtestDecaySingle() {
	start := mustParseTime("2024-02-01")
	end := mustParseTime("2025-09-15")
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
	result := executeBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func backtestDecayHeatmaps() {
	start := mustParseTime("2024-01-01")
	end := mustParseTime("2025-09-15")
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
	results := commons.ParallelMap(strategies, func (strategy decayStrategy) StrategyResult {
		result := executeBacktest(&strategy, start, end, historyMap, dailyData, prices)
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
	start := mustParseTime("2024-01-01")
	end := mustParseTime("2025-09-15")
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
	result := executeBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func backtestJump() {
	start := mustParseTime("2024-10-01")
	end := mustParseTime("2025-09-15")
	includeTags := []string{
		"politics",
		/*
		"politics",
		"geopolitics",
		"world",
		"trump",
		"trump-presidency",
		*/
	}
	excludeTags := []string{
		"crypto",
		"sports",
		"games",
		"mention-markets",
	}
	const (
		threshold1 = 0.3
		threshold2 = 0.5
		threshold3 = 0.75
		stopLoss = false
		positionSize = 250.0
		holdingTime = 24
	)
	strategy := jumpStrategy{
		includeTags: includeTags,
		excludeTags: excludeTags,
		threshold1: threshold1,
		threshold2: threshold2,
		threshold3: threshold3,
		stopLoss: stopLoss,
		positionSize: positionSize,
		holdingTime: holdingTime,
		previousPrices: map[string]priceSample{},
	}
	historyMap, dailyData, prices := loadBacktestData()
	result := executeBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func backtestMention() {
	start := mustParseTime("2024-10-01")
	end := mustParseTime("2025-06-15")
	const (
		threshold1 = 0.3
		threshold2 = 0.7
		minSamples = 24
		positionSize = 75.0
	)
	strategy := mentionStrategy{
		threshold1: threshold1,
		threshold2: threshold2,
		minSamples: minSamples,
		positionSize: positionSize,
		sampleCounts: map[string]int{},
	}
	historyMap, dailyData, prices := loadBacktestData()
	result := executeBacktest(&strategy, start, end, historyMap, dailyData, prices)
	result.print()
	dailyEquityCurve := getDailyEquityCurve(result.equityCurve)
	plotData("equity", dailyEquityCurve)
}

func plotData(argument string, data any) {
	arguments := []string{
		"python/plot.py",
		argument,
	}
	commons.PythonPipe(arguments, data)
}