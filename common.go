package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/encratite/commons"
	"github.com/shopspring/decimal"
)

type keyValuePair[K comparable, V any] struct {
	key K
	value V
}

func getJSON[T any](base string, parameters map[string]string) (T, error) {
	u, err := url.Parse(base)
	if err != nil {
		log.Fatalf("Unable to parse URL (%s): %v", base, err)
	}
	values := url.Values{}
	for key, value := range parameters {
		values.Add(key, value)
	}
	u.RawQuery = values.Encode()
	encoded := u.String()
	// log.Printf("URL: %s", encoded)
	response, err := http.Get(encoded)
	var empty T
	if err != nil {
		log.Printf("Failed to GET data (%s): %v", encoded, err)
		return empty, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response (%s): %v", encoded, err)
		return empty, err
	}
	var output T
	err = json.Unmarshal(body, &output)
	if err != nil {
		log.Printf("Failed to parse JSON data (%s): %v", encoded, err)
		log.Print(string(body))
		return empty, err
	}
	return output, nil
}

func decimalConstant(s string) decimal.Decimal {
	output, err := decimal.NewFromString(s)
	if err != nil {
		log.Fatalf("Failed to convert string \"%s\" to decimal: %v", s, err)
	}
	return output
}

func sortMapByKey[K comparable, V any](m map[K]V, compare func (K, K) int) []V {
	pairs := []keyValuePair[K, V]{}
	for key, value := range m {
		pair := keyValuePair[K, V]{
			key: key,
			value: value,
		}
		pairs = append(pairs, pair)
	}	
	slices.SortFunc(pairs, func (a, b keyValuePair[K, V]) int {
		return compare(a.key, b.key)
	})
	values := []V{}
	for _, pair := range pairs {
		values = append(values, pair.value)
	}
	return values
}

func sortMapByValue[K comparable, V any](m map[K]V, compare func (V, V) int) []V {
	pairs := []keyValuePair[K, V]{}
	for key, value := range m {
		pair := keyValuePair[K, V]{
			key: key,
			value: value,
		}
		pairs = append(pairs, pair)
	}	
	slices.SortFunc(pairs, func (a, b keyValuePair[K, V]) int {
		return compare(a.value, b.value)
	})
	values := []V{}
	for _, pair := range pairs {
		values = append(values, pair.value)
	}
	return values
}

func getRateOfChange(newValue, oldValue float64) float64 {
	return newValue / oldValue - 1.0
}

func mustParseTime(timeString string) time.Time {
	return commons.MustParseTime(timeString)
}