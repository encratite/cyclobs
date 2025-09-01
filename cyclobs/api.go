package cyclobs

import (
	"fmt"
	"strconv"
)

func getEvents(tagSlug string) ([]Event, error) {
	url := "https://gamma-api.polymarket.com/events/pagination"
	parameters := map[string]string{
		"limit": strconv.FormatInt(eventsLimit, 10),
		"archived": "false",
		"tag_slug": tagSlug,
		"order": "volume24hr",
		"ascending": "false",
	}
	events, err := getJSON[EventsResponse](url, parameters)
	if err != nil {
		return nil, err
	}
	return events.Data, nil
}

func getMarket(slug string) (Market, error) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/markets/slug/%s", slug)
	market, err := getJSON[Market](url, map[string]string{})
	if err != nil {
		return Market{}, err
	}
	return market, nil
}

func getPositions() ([]Position, error) {
	url := "https://data-api.polymarket.com/positions"
	parameters := map[string]string{
		"user": configuration.Credentials.ProxyAddress,
	}
	positions, err := getJSON[[]Position](url, parameters)
	if err != nil {
		return nil, err
	}
	return positions, nil
}