package cyclobs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

const (
	dateLayout = "2006-01-02"
	timestampLayout = "2006-01-02 15:04:05"
)

type keyValuePair[K comparable, V any] struct {
	key K
	value V
}

type taskTuple[T any] struct {
	index int
	element T
}

func readFile(path string) []byte {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file (%s): %v", path, err)
	}
	return content
}

func contains[T comparable](slice []T, element T) bool {
	for _, x := range slice {
		if x == element {
			return true
		}
	}
	return false
}

func containsFunc[T any](slice []T, match func (T) bool) bool {
	for _, x := range slice {
		if match(x) {
			return true
		}
	}
	return false
}

func find[T any](slice []T, match func (T) bool) (T, bool) {
	index := slices.IndexFunc(slice, func (element T) bool {
		return match(element)
	})
	if index >= 0 {
		return slice[index], true
	} else {
		var zeroValue T
		return zeroValue, false
	}
}

func findPointer[T any](slice []T, match func (T) bool) (*T, bool) {
	index := slices.IndexFunc(slice, func (element T) bool {
		return match(element)
	})
	if index >= 0 {
		return &slice[index], true
	} else {
		return nil, false
	}
}

func intToString(integer int64) string {
	return strconv.FormatInt(integer, 10)
}

func parseISOTime(timeString string) (time.Time, error) {
	timestamp, err := time.Parse(time.RFC3339, timeString)
	if err != nil {
		return time.Time{}, err
	}
	return timestamp, nil
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

func getDate(timestamp time.Time) time.Time {
	return time.Date(
		timestamp.Year(),
		timestamp.Month(),
		timestamp.Day(),
		0,
		0,
		0,
		0,
		timestamp.Location(),
	)
}

func getHourTimestamp(timestamp time.Time) time.Time {
	return time.Date(
		timestamp.Year(),
		timestamp.Month(),
		timestamp.Day(),
		timestamp.Hour(),
		0,
		0,
		0,
		timestamp.Location(),
	)
}

func getDateFromString(dateString string) time.Time {
	date, err := time.Parse(dateLayout, dateString)
	if err != nil {
		log.Fatalf("failed to parse date string \"%s\": %v", dateString, err)
	}
	return date
}

func getDateString(date time.Time) string {
	return date.Format(dateLayout)
}

func getReturns(newValue, oldValue float64) float64 {
	return newValue / oldValue - 1.0
}

func writeFile(path, data string) {
	bytes := []byte(data)
	err := os.WriteFile(path, bytes, 0644)
	if err != nil {
		log.Fatalf("Failed to write file (%s): %v", path, err)
	}
}

func getTimeString(timestamp time.Time) string {
	return timestamp.Format(timestampLayout)
}

func getRateOfChange(newValue, oldValue float64) float64 {
	return newValue / oldValue - 1.0
}

func formatMoney(amount float64) string {
	amountString := fmt.Sprintf("%d", int64(amount))
	output := "$"
	for i, character := range amountString {
		if i > 0 && (len(amountString) - i) % 3 == 0 {
			output += ","
		}
		output += string(character)
	}
	return output
}

func parallelMap[A, B any](elements []A, callback func(A) B) []B {
	workers := runtime.NumCPU()
	elementChan := make(chan taskTuple[A], len(elements))
	for i, x := range elements {
		elementChan <- taskTuple[A]{
			index: i,
			element: x,
		}
	}
	close(elementChan)
	var wg sync.WaitGroup
	wg.Add(workers)
	output := make([]B, len(elements))
	for range workers {
		go func() {
			defer wg.Done()
			for task := range elementChan {
				output[task.index] = callback(task.element)
			}
		}()
	}
	wg.Wait()
	return output
}