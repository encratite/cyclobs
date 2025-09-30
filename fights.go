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
)

const (
	fightPriceOffsetHours = 24
	winThreshold = 0.95
	minFightSize = 4 * 1024
)

type fightResult struct {
	price float64
	outcome bool
}

type priceData struct {
	timestamp time.Time
	price float64
}

func analyzeFights(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		log.Fatalf("Unable to read directory: %s", directory)
	}
	results := []fightResult{}
	for _, entry := range entries {
		if entry.IsDir() {
			path := filepath.Join(directory, entry.Name())
			fightResults := loadFightDirectory(path)
			results = append(results, fightResults...)
		}
	}
	bigBins := []outcomeStats{
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
	smallBins := []outcomeStats{
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
}

func loadFightDirectory(directory string) []fightResult {
	entries, err := os.ReadDir(directory)
	if err != nil {
		log.Fatalf("Unable to read directory: %s", directory)
	}
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
				paths = append(paths, path)
			}
		}
	}
	pointers := commons.ParallelMap(paths, loadFight)
	results := []fightResult{}
	for _, pointer := range pointers {
		if pointer != nil {
			results = append(results, *pointer)
		}
	}
	return results
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
	var fightStart *time.Time = nil
	for i := lastIndex; i >= 0; i-- {
		price := prices[i]
		if price.price > 1.0 - winThreshold && price.price < winThreshold {
			start := price.timestamp.Add(time.Duration(-1) * time.Hour)
			fightStart = &start
			break
		}
	}
	if fightStart == nil {
		log.Printf("Warning: failed to determine start of fight %s", path)
		return nil
	}
	for i := lastIndex; i >= 0; i-- {
		price := prices[i]
		delta := fightStart.Sub(price.timestamp)
		if delta > minDuration {
			outcome := lastPrice.price > winThreshold
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