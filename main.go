package main

import (
	"flag"

	"cyclobs/cyclobs"
)

func main() {
	dataMode := flag.Bool("data", false, "Run data collection mode without any automated trading")
	triggerMode := flag.Bool("trigger", false, "Run automated trading system using take profit/stop-loss trigger levels defined in the configuration file")
	history := flag.Bool("history", false, "Download recent historical data")
	analyze := flag.Bool("analyze", false, "Analyze historical data previously downloaded using -history")
	download := flag.String("download", "", "Download the complete price history of the specified event slug, also requires -output")
	output := flag.String("output", "", "The directory to download the complete price history to, only works in combination with -download")
	screener := flag.Bool("screener", false, "Filter for events that meet certain criteria")
	backtest := flag.Bool("backtest", false, "Run backtest")
	flag.Parse()
	if *dataMode {
		cyclobs.DataMode()
	} else if *triggerMode {
		cyclobs.TriggerMode()
	} else if *history {
		cyclobs.History()
	} else if *analyze {
		cyclobs.Analyze()
	} else if *download != "" && *output != "" {
		cyclobs.Download(*download, *output)
	} else if *screener {
		cyclobs.Screener()
	} else if *backtest {
		cyclobs.Backtest()
	} else {
		flag.Usage()
	}
}