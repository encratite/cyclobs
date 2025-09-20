package cyclobs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	historyFidelitySingle = 1
)

func Download(slug string, directory string) {
	event, err := getEventBySlug(slug)
	if err != nil {
		return
	}
	os.MkdirAll(directory, 0755)
	for _, market := range event.Markets {
		startDate, err := parseISOTime(market.StartDate)
		if err != nil {
			log.Fatalf("Unable to parse start date: \"%s\"", market.StartDate)
			return
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
			output += fmt.Sprintf("%s,%g\n", getTimeString(timestamp), sample.Price)
		}
		path := filepath.Join(directory, fmt.Sprintf("%s.csv", market.Slug))
		writeFile(path, output)
		log.Printf("Downloaded %d samples to %s", len(history.History), path)
	}
}