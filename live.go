package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/encratite/commons"
	"gonum.org/v1/gonum/stat"
)

const (
	liveTriggerLevel = 0.75
	liveWinThreshold = 0.99
	liveRaceDuration = 10
	liveMinRecords = 10
)

type liveRecord struct {
	timestamp time.Time
	price float64
}

func evaluateLiveBetting(directory string) {
	directories := commons.GetDirectories(directory)
	wins := 0
	total := len(directories)
	prices := []float64{}
	for _, path := range directories {
		win, price := evaluateLiveData(path)
		if win {
			wins++
		}
		if price != nil && *price >= liveTriggerLevel {
			prices = append(prices, *price)
		}
	}
	winRatio := 100.0 * float64(wins) / float64(total)
	meanPrice := stat.Mean(prices, nil)
	fmt.Printf("Trigger level: %.2f\n", liveTriggerLevel)
	fmt.Printf("Wins: %.1f%% (%d out of %d)\n", winRatio, wins, total)
	fmt.Printf("Next fill: %.3f\n", meanPrice)
}

func evaluateLiveData(directory string) (bool, *float64) {
	files := commons.GetFiles(directory, ".csv")
	var earliestTrigger *time.Time
	var earliestTriggerNextPrice *float64
	var earliestWin *bool
	for _, path := range files {
		if !strings.Contains(path, "-buy.csv") {
			continue
		}
		win, triggerTime, nextPrice := getEarliestTriggerData(path)
		if triggerTime == nil {
			continue
		}
		if earliestTrigger == nil || triggerTime.Before(*earliestTrigger) {
			earliestTrigger = triggerTime
			earliestTriggerNextPrice = nextPrice
			earliestWin = &win
		}
	}
	if earliestTrigger == nil {
		log.Fatalf("Unable to determine the outcome for event: %s", directory)
	}
	return *earliestWin, earliestTriggerNextPrice
}

func getEarliestTriggerData(path string) (bool, *time.Time, *float64) {
	liveRecords := []liveRecord{}
	commons.ReadCsv(path, func (records []string) {
		timestamp := commons.MustParseTime(records[0])
		price := commons.MustParseFloat(records[1])	
		record := liveRecord{
			timestamp: timestamp,
			price: price,
		}
		liveRecords = append(liveRecords, record)
	})
	if len(liveRecords) < liveMinRecords {
		return false, nil, nil
	}
	var triggerTime *time.Time
	var nextPrice *float64
	finalValue := 0.0
	lastTimestamp := liveRecords[len(liveRecords) - 1].timestamp
	raceDuration := time.Duration(liveRaceDuration) * time.Hour
	for i, record := range liveRecords {
		delta := lastTimestamp.Sub(record.timestamp)
		if delta > raceDuration {
			continue
		}
		if triggerTime == nil && record.price >= liveTriggerLevel {
			triggerTime = &record.timestamp
			nextIndex := i + 1
			if nextIndex < len(liveRecords) {
				nextPrice = &liveRecords[nextIndex].price
			}
		}
		finalValue = record.price
	}
	win := finalValue >= liveWinThreshold
	return win, triggerTime, nextPrice
}