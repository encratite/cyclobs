//go:build windows
// +build windows

package cyclobs

import (
	"golang.org/x/sys/windows"
)

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