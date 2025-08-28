package cyclobs

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"

	"github.com/shopspring/decimal"
	"golang.org/x/sys/windows"
)

func readFile(path string) []byte {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file (%s): %v", path, err)
	}
	return content
}

func contains[T comparable](slice []T, value T) bool {
	for _, x := range slice {
		if x == value {
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

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procBeep = kernel32.NewProc("Beep")
)

func beep() {
	go func() {
		frequency := 900
		duration := 800
		procBeep.Call(uintptr(frequency), uintptr(duration))
	}()
}

func intToString(integer int64) string {
	return strconv.FormatInt(integer, 10)
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
		log.Printf("Failed to GET markets (%s): %v", encoded, err)
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