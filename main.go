package main

import (
	"flag"
)

func main() {
	dataMode := flag.Bool("data", false, "Run data collection mode without any automated trading")
	triggerMode := flag.Bool("trigger", false, "Run automated trading system using take profit/stop-loss trigger levels defined in the configuration file")
	jump := flag.Bool("jump", false, "Run automated trading system using the jump strategy")
	earnings := flag.Bool("earnings", false, "Run earnings watcher")
	history := flag.Bool("history", false, "Download recent historical data")
	analyze := flag.Bool("analyze", false, "Analyze historical data previously downloaded using -history")
	download := flag.String("download", "", "Download the complete price history of the specified event slug, also requires -output")
	trades := flag.String("trades", "", "Download historical trades from the event to the target directory specified by -output")
	output := flag.String("output", "", "The directory to download the complete price history to, only works in combination with -download")
	screener := flag.Bool("screener", false, "Filter for events that meet certain criteria")
	backtest := flag.Bool("backtest", false, "Run backtest")
	tags := flag.String("tags", "", "Get the tags of an event")
	relatedTags := flag.String("related", "", "Find related tags")
	outcomes := flag.Bool("outcomes", false, "Analyze the correlation between prices and outcomes")
	fights := flag.String("fights", "", "Analyze outcomes of boxing matches or UFC fights")
	live := flag.String("live", "", "Evaluate live betting trigger levels using on-chain data in the specified directory")
	flag.Parse()
	if *dataMode {
		runMode(systemDataMode)
	} else if *triggerMode {
		runMode(systemTriggerMode)
	} else if *jump {
		runJumpSystem()
	} else if *earnings {
		runEarningsSystem()
	} else if *history {
		updateHistory()
	} else if *analyze {
		analyzeData()
	} else if *download != "" && *output != "" {
		downloadEvent(*download, *output)
	} else if *trades != "" && *output != "" {
		downloadTrades(*trades, *output)
	} else if *screener {
		runScreener()
	} else if *backtest {
		runBacktest()
	} else if *tags != "" {
		showEventTags(*tags)
	} else if *relatedTags != "" {
		showRelatedTags(*relatedTags)
	} else if *outcomes {
		analyzeOutcomes()
	} else if *fights != "" {
		analyzeFights(*fights)
	} else if *live != "" {
		evaluateLiveBetting(*live)
	} else {
		flag.Usage()
	}
}