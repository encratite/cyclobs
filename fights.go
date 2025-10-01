package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/encratite/commons"
	"gonum.org/v1/gonum/stat"
)

const (
	fightPriceOffsetHours = 24
	winThreshold = 0.95
	minFightSize = 4 * 1024
	fightPriceFilterMin = 0.45
	fightPriceFilterMax = 0.55
	printFilteredFights = true
)

// var fightCutoffDate time.Time = commons.MustParseTime("2025-07-01")
var fightCutoffDate time.Time = time.Time{}

type fightOutcome int

const (
	outcomeWin fightOutcome = iota
	outcomeLoss
	outcomeDraw
)

type fightResult struct {
	price float64
	outcome fightOutcome
}

type priceData struct {
	timestamp time.Time
	price float64
}

type filteredFight struct {
	event string
	market string
	price float64
	outcome fightOutcome
}

type fightOutcomeBin struct {
	range1 float64
	range2 float64
	prices []float64
	wins int
	losses int
	draws int
}

func analyzeFights(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		log.Fatalf("Unable to read directory: %s", directory)
	}
	results := []fightResult{}
	filteredFights := []filteredFight{}
	for _, entry := range entries {
		if entry.IsDir() {
			event := entry.Name()
			path := filepath.Join(directory, event)
			fightResults, filtered := loadFightDirectory(event, path)
			results = append(results, fightResults...)
			filteredFights = append(filteredFights, filtered...)
		}
	}
	bigBins := []fightOutcomeBin{
		{
			range1: 0.0,
			range2: 0.3,
		},
		{
			range1: 0.4,
			range2: 0.5,
		},
		{
			range1: 0.5,
			range2: 0.6,
		},
		{
			range1: 0.6,
			range2: 0.7,
		},
		{
			range1: 0.7,
			range2: 1.0,
		},
	}
	smallBins := []fightOutcomeBin{
		{
			range1: 0.4,
			range2: 0.45,
		},
		{
			range1: 0.45,
			range2: 0.5,
		},
		{
			range1: 0.5,
			range2: 0.55,
		},
		{
			range1: 0.55,
			range2: 0.6,
		},
		{
			range1: 0.6,
			range2: 0.65,
		},
	}
	for _, result := range results {
		for i := range bigBins {
			currentStats := &bigBins[i]
			currentStats.add(result.price, result.outcome)
		}
		for i := range smallBins {
			currentStats := &smallBins[i]
			currentStats.add(result.price, result.outcome)
		}
	}
	fmt.Printf("Big bins:\n")
	for _, currentStats := range bigBins {
		currentStats.print()
	}
	fmt.Printf("\nSmall bins:\n")
	for _, currentStats := range smallBins {
		currentStats.print()
	}
	fmt.Printf("\nFiltered fights:\n")
	if printFilteredFights {
		for _, fight := range filteredFights {
			outcomeString := getOutcomeString(fight.outcome)
			fmt.Printf("\tevent = %s, market = %s, price = %.2f, outcome = %s\n", fight.event, fight.market, fight.price, outcomeString)
		}
	}
}

func loadFightDirectory(event string, directory string) ([]fightResult, []filteredFight) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		log.Fatalf("Unable to read directory: %s", directory)
	}
	markets := []string{}
	paths := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && filepath.Ext(name) == ".csv" && !strings.Contains(name, "-x-") && !strings.Contains(name, "-y-") {
			path := filepath.Join(directory, name)
			stats, err := os.Stat(path)
			if err != nil {
				log.Fatalf("Failed to retrieve stats for file: %s", path)
			}
			if stats.Size() > minFightSize {
				market := strings.TrimSuffix(name, filepath.Ext(name))
				markets = append(markets, market)
				paths = append(paths, path)
			}
		}
	}
	pointers := commons.ParallelMap(paths, loadFight)
	results := []fightResult{}
	filteredFights := []filteredFight{}
	for i, pointer := range pointers {
		if pointer != nil {
			results = append(results, *pointer)
			price := pointer.price
			name := markets[i]
			if price >= fightPriceFilterMin && price < fightPriceFilterMax {
				fight := filteredFight{
					event: event,
					market: name,
					price: price,
					outcome: pointer.outcome,
				}
				filteredFights = append(filteredFights, fight)
			}
		}
	}
	return results, filteredFights
}

func loadFight(path string) *fightResult {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to read fighter data: %v", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	_, _ = reader.Read()
	prices := []priceData{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		timestamp := commons.MustParseTime(record[0])
		price := commons.MustParseFloat(record[1])
		data := priceData{
			timestamp: timestamp,
			price: price,
		}
		prices = append(prices, data)
	}
	lastIndex := len(prices) - 1
	lastPrice := prices[lastIndex]
	minDuration := time.Duration(fightPriceOffsetHours) * time.Hour
	var fightEnd *time.Time = nil
	for i := lastIndex; i >= 0; i-- {
		price := prices[i]
		if price.price > 1.0 - winThreshold && price.price < winThreshold {
			start := price.timestamp.Add(time.Duration(-1) * time.Hour)
			fightEnd = &start
			break
		}
	}
	if fightEnd == nil {
		log.Printf("Warning: failed to determine end of fight %s", path)
		return nil
	}
	if !fightCutoffDate.Equal(time.Time{}) && fightEnd.Before(fightCutoffDate) {
		return nil
	}
	for i := lastIndex; i >= 0; i-- {
		price := prices[i]
		delta := fightEnd.Sub(price.timestamp)
		if delta > minDuration {
			outcome := outcomeDraw
			if lastPrice.price > winThreshold {
				outcome = outcomeWin
			} else if lastPrice.price < 1.0 - winThreshold {
				outcome = outcomeLoss
			}
			result := fightResult{
				price: price.price,
				outcome: outcome,
			}
			return &result
		}
	}
	// log.Printf("Warning: failed to determine price in %s", path)
	return nil
}

func (s *fightOutcomeBin) add(price float64, outcome fightOutcome) {
	if price >= s.range1 && price < s.range2 {
		s.prices = append(s.prices, price)
		switch outcome {
		case outcomeWin:
			s.wins++
		case outcomeLoss:
			s.losses++
		case outcomeDraw:
			s.draws++
		}
	}
}

func (s *fightOutcomeBin) print() {
	samples := len(s.prices)
	total := float64(samples)
	winRatio := float64(s.wins) / total
	lossRatio := float64(s.losses) / total
	drawRatio := float64(s.draws) / total
	meanPrice := stat.Mean(s.prices, nil)
	adjustedWinRatio := winRatio / (winRatio + lossRatio)
	delta := meanPrice - adjustedWinRatio
	format := "\t%.2f - %.2f: delta = %.2f, wins = %.1f%%, losses = %.1f%%, draws = %.1f%%, samples = %d, meanPrice = %.2f\n"
	fmt.Printf(
		format,
		s.range1,
		s.range2,
		delta,
		percent * winRatio,
		percent * lossRatio,
		percent * drawRatio,
		samples,
		meanPrice,
	)
}

func getOutcomeString(outcome fightOutcome) string {
	switch outcome {
	case outcomeWin:
		return "win"
	case outcomeLoss:
		return "loss"
	case outcomeDraw:
		return "draw"
	}
	return "?"
}