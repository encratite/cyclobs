package main

import (
	"flag"

	"cyclobs/cyclobs"
)

func main() {
	dataMode := flag.Bool("data", false, "Run data collection mode without any automated trading")
	triggerMode := flag.Bool("trigger", false, "Run automated trading system using take profit/stop-loss trigger levels defined in the configuration file")
	flag.Parse()
	if *dataMode {
		cyclobs.DataMode()
	} else if *triggerMode {
		cyclobs.TriggerMode()
	} else {
		flag.Usage()
	}
}