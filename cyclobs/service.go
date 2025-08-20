package cyclobs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const gammaURL = "https://gamma-api.polymarket.com"
const eventsLimit = 50

func RunService() {
	loadConfiguration()
	events := getEvents("politics")
	if events != nil {
		fmt.Printf("Received %d events", len(events))
	}
}

func getEvents(tagSlug string) []Event {
	base := fmt.Sprintf("%s/events/pagination", gammaURL)
	u, err := url.Parse(base)
	if err != nil {
		log.Fatalf("Unable to parse URL (%s): %v", base, err)
	}
	values := url.Values{}
	values.Add("limit", strconv.FormatInt(eventsLimit, 10))
	values.Add("archived", "false")
	values.Add("tag_slug", tagSlug)
	values.Add("order", "volume24hr")
	values.Add("ascending", "false")
	u.RawQuery = values.Encode()
	encoded := u.String()
	response, err := http.Get(encoded)
	if err != nil {
		log.Printf("Failed to GET markets (%s): %v", encoded, err)
		return nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response (%s): %v", encoded, err)
		return nil
	}
	var eventsResponse EventsResponse
	err = json.Unmarshal(body, &eventsResponse)
	if err != nil {
		log.Printf("Failed to parse market JSON data (%s): %v", encoded, err)
		return nil
	}
	return eventsResponse.Data
}