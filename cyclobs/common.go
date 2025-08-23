package cyclobs

import (
	"log"
	"os"
	"slices"
	"strconv"

	"golang.org/x/sys/windows"
)

func readFile(path string) []byte {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file (%s): %v", path, err)
	}
	return content
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
	frequency := 900
	duration := 800
	procBeep.Call(uintptr(frequency), uintptr(duration))
}

func intToString(integer int64) string {
	return strconv.FormatInt(integer, 10)
}