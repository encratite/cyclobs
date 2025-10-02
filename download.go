package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/encratite/commons"
)

const (
	historyFidelitySingle = 1
)

func downloadEvent(slug string, directory string) {
	event, err := getEventBySlug(slug)
	if err != nil {
		return
	}
	commons.CreateDirectory(directory)
	for _, market := range event.Markets {
		startDate, err := commons.ParseTime(market.StartDate)
		if err != nil {
			createdDate, err := commons.ParseTime(market.CreatedAt)
			if err != nil {
				log.Fatalf("Unable to parse start date: \"%s\"/\"%s\"", market.StartDate, market.CreatedAt)
				return
			}
			startDate = createdDate
		}
		yesID, err := getCLOBTokenID(market, true)
		if err != nil {
			return
		}
		history, err := getPriceHistory(yesID, startDate, historyFidelitySingle)
		if err != nil {
			return
		}
		output := "time,price\n"
		for _, sample := range history.History {
			timestamp := time.Unix(int64(sample.Time), 0).UTC()
			output += fmt.Sprintf("%s,%g\n", commons.GetTimeString(timestamp), sample.Price)
		}
		path := filepath.Join(directory, fmt.Sprintf("%s.csv", market.Slug))
		commons.WriteFile(path, output)
		log.Printf("Downloaded %d samples to %s", len(history.History), path)
	}
}