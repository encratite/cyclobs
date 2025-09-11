package cyclobs

import (
	"fmt"
)

const (
	backtestSamplingHour = 15
)

type backtestDailyData struct {
	historyData []PriceHistoryBSON
}

func Backtest() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	closed := true
	negRisk := false
	minVolume := 100000.0
	historyData := database.getPriceHistoryData(&closed, &negRisk, &minVolume)
	fmt.Printf("Length: %d\n", len(historyData))
}