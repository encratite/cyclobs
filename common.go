package main

import (
	"log"
	"slices"
	"time"

	"github.com/encratite/commons"
	"github.com/shopspring/decimal"
)

type keyValuePair[K comparable, V any] struct {
	key K
	value V
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