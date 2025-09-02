package cyclobs

import (
	"cmp"
	"fmt"
	"log"
	"slices"
)

type outcomeCount struct {
	tag string
	yes int
	no int
	total int
}

func Analyze() {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	historyData := database.getPriceHistoryData()
	log.Printf("Samples: %d", len(historyData))
	outcomeMap := map[string]outcomeCount{}
	for _, history := range historyData {
		if !history.Closed || history.Outcome == nil {
			continue
		}
		for _, tag := range history.Tags {
			count, exists := outcomeMap[tag]
			if !exists {
				count = outcomeCount{
					tag: tag,
					yes: 0,
					no: 0,
					total: 0,
				}
			}
			if *history.Outcome {
				count.yes++
			} else {
				count.no++
			}
			count.total++
			outcomeMap[tag] = count
		}
	}
	outcomes := []outcomeCount{}
	for _, outcome := range outcomeMap {
		outcomes = append(outcomes, outcome)
	}
	slices.SortFunc(outcomes, func (a, b outcomeCount) int {
		return cmp.Compare(a.total, b.total)
	})
	for _, outcome := range outcomes {
		percentage := float64(outcome.yes) / float64(outcome.total) * 100.0
		fmt.Printf("%s: %.1f%% (%d samples)\n", outcome.tag, percentage, outcome.total)
	}
}