package cyclobs

import (
	"cmp"
	"fmt"
	"log"
	"strconv"
)

type tagCount struct {
	tag string
	count int
}

func EventTags(slug string) {
	event, err := getEventBySlug(slug)
	if err != nil {
		log.Fatalf("Failed to get event %s: %v", slug, err)
	}
	id, err := strconv.Atoi(event.ID)
	if err != nil {
		log.Fatalf("Failed to parse event ID: %s", event.ID)
	}
	eventTags, err := getEventTags(id)
	if err != nil {
		log.Fatalf("Failed to get event tags for %s: %v", slug, err)
	}
	fmt.Printf("Tags:\n")
	for i, tag := range eventTags {
		fmt.Printf("\t%d. %s\n", i + 1, tag.Slug)
	}
}

func RelatedTags(tag string) {
	loadConfiguration()
	database := newDatabaseClient()
	defer database.close()
	historyData := database.getTagsOnly()
	countMap := map[string]tagCount{}
	total := 0
	for _, history := range historyData {
		if contains(history.Tags, tag) {
			for _, t := range history.Tags {
				if t != tag {
					count, exists := countMap[t]
					if !exists {
						count = tagCount{
							tag: t,
							count: 0,
						}
					}
					count.count++
					countMap[t] = count
					total++
				}
			}
		}
	}
	counts := sortMapByValue(countMap, func (a, b tagCount) int {
		return cmp.Compare(b.count, a.count)
	})
	for i, count := range counts {
		if i >= 50 {
			break
		}
		percentage := float64(count.count) / float64(total) * percent
		fmt.Printf("\t%d. %s: %.1f%% (%d total)\n", i + 1, count.tag, percentage, count.count)
	}
}