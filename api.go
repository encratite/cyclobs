package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/encratite/commons"
)

const (
	tradesAPILimit = 500
	tradesAPIOffsetLimit = 1000
	activityAPILimit = 500
)

func getEvents(tagSlug *string) ([]Event, error) {
	url := "https://gamma-api.polymarket.com/events/pagination"
	parameters := map[string]string{
		"limit": strconv.FormatInt(eventsLimit, 10),
		"archived": "false",
		"order": "volume24hr",
		"ascending": "false",
	}
	if tagSlug != nil {
		parameters["tag_slug"] = *tagSlug
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
		"offset": commons.IntToString(offset),
		"limit": commons.IntToString(limit),
		"order": order,
		"ascending": "false",
		"start_date_min": startDateMin,
	}
	if tagID != nil {
		parameters["tag_id"] = commons.IntToString(*tagID)
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
		"startTs": commons.Int64ToString(unixTimestamp),
		"fidelity": commons.IntToString(fidelity),
	}
	history, err := getJSON[PriceHistory](url, parameters)
	if err != nil {
		return PriceHistory{}, err
	}
	return history, nil
}

func getTrades(conditionID string, offset int) ([]Trade, error) {
	url := "https://data-api.polymarket.com/trades"
	parameters := map[string]string{
		"market": conditionID,
		"limit": commons.IntToString(tradesAPILimit),
		"offset": commons.IntToString(offset),
		"filterType": "CASH",
		"filterAmount": "1",
	}
	trades, err := getJSON[[]Trade](url, parameters)
	return trades, err
}

func getActivities(proxyWallet string, offset int, start time.Time, end time.Time) ([]Activity, error) {
	url := "https://data-api.polymarket.com/activity"
	parameters := map[string]string{
		"user": proxyWallet,
		"limit": commons.IntToString(activityAPILimit),
		"offset": commons.IntToString(offset),
		"start": commons.Int64ToString(start.UTC().Unix()),
		"end": commons.Int64ToString(end.UTC().Unix()),
		"sortBy": "TIMESTAMP",
		"sortDirection": "ASC",
	}
	activities, err := getJSON[[]Activity](url, parameters)
	return activities, err
}

func getTag(slug string) (Tag, error) {
	url := fmt.Sprintf("https://gamma-api.polymarket.com/tags/slug/%s", slug)
	parameters := map[string]string{}
	tag, err := getJSON[Tag](url, parameters)
	return tag, err
}