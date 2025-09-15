package cyclobs

import (
	"fmt"
	"strconv"
	"time"
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

func getEventBySlug(slug string) (Event, error) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/events/slug/%s", slug)
	parameters := map[string]string{
		"include_chat": "false",
		"include_template": "false",
	}
	event, err := getJSON[Event](url, parameters)
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

func getEventTags(id int) ([]EventTag, error) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/events/%d/tags", id)
	tags, err := getJSON[[]EventTag](url, map[string]string{})
	if err != nil {
		return []EventTag{}, err
	}
	return tags, nil
}

func getMarket(slug string) (Market, error) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/markets/slug/%s", slug)
	market, err := getJSON[Market](url, map[string]string{})
	if err != nil {
		return Market{}, err
	}
	return market, nil
}

func getMarkets(offset, limit int, order, startDateMin string, tagID *int) ([]Market, error) {
	url := "https://gamma-api.polymarket.com/markets"
	parameters := map[string]string{
		"offset": intToString(int64(offset)),
		"limit": intToString(int64(limit)),
		"order": order,
		"ascending": "false",
		"start_date_min": startDateMin,
	}
	if tagID != nil {
		parameters["tag_id"] = intToString(int64(*tagID))
	}
	markets, err := getJSON[[]Market](url, parameters)
	if err != nil {
		return []Market{}, err
	}
	return markets, nil
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

func getPriceHistory(market string, start time.Time, fidelity int) (PriceHistory, error) {
	unixTimestamp := start.Unix()
	url := "https://clob.polymarket.com/prices-history"
	parameters := map[string]string{
		"market": market,
		"startTs": intToString(unixTimestamp),
		"fidelity": intToString(int64(fidelity)),
	}
	history, err := getJSON[PriceHistory](url, parameters)
	if err != nil {
		return PriceHistory{}, err
	}
	return history, nil
}