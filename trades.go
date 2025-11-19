package main

import (
	"cmp"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"time"

	"github.com/encratite/commons"
	"github.com/encratite/gamma"
)

const (
	tradesDownloadThrottle = 100
	fileExistsCheck = false
)

func downloadTrades(slug, directory string) {
	outputDirectory := filepath.Join(directory, slug)
	commons.CreateDirectory(outputDirectory)
	event, err := gamma.GetEventBySlug(slug)
	if err != nil {
		return
	}
	for _, market := range event.Markets {
		yesID, err := getCLOBTokenID(market, true)
		if err != nil {
			log.Fatal(err)
		}
		downloadMarketTrades(market.Slug, market.ConditionID, yesID, outputDirectory)
	}
}

func downloadMarketTrades(slug, conditionID, yesID, directory string) {
	buyFileName := fmt.Sprintf("%s-buy.csv", slug)
	sellFileName := fmt.Sprintf("%s-sell.csv", slug)
	buyOutputPath := filepath.Join(directory, buyFileName)
	sellOutputPath := filepath.Join(directory, sellFileName)
	if fileExistsCheck && commons.FileExists(buyOutputPath) {
		log.Printf("%s already exists, skipping\n", buyOutputPath)
		return
	}
	buys := []gamma.Trade{}
	sells := []gamma.Trade{}
	for offset := 0; offset <= gamma.TradesAPIOffsetLimit; offset += gamma.TradesAPILimit {
		trades, err := gamma.GetTrades(conditionID, offset)
		if err != nil {
			return
		}
		for _, trade := range trades {
			if trade.Asset != yesID {
				continue
			}
			if trade.Side == "BUY" {
				buys = append(buys, trade)
			} else {
				sells = append(sells, trade)
			}
		}
		lastTimestampString := "N/A"
		if len(trades) > 0 {
			lastTrade := trades[len(trades) - 1]
			lastTimestamp := time.Unix(lastTrade.Timestamp, 0)
			lastTimestampString = commons.GetTimeString(lastTimestamp)
		}
		log.Printf("Downloaded data: slug = %s, offset = %d, trades = %d, lastTimestamp = %s", slug, offset, len(trades), lastTimestampString)
		if len(trades) < gamma.TradesAPILimit {
			break
		}
		time.Sleep(time.Duration(tradesDownloadThrottle) * time.Millisecond)
	}
	writeTradesToFile(buys, buyOutputPath)
	writeTradesToFile(sells, sellOutputPath)
}

func writeTradesToFile(trades []gamma.Trade, path string) {
	if len(trades) == 0 {
		return
	}
	slices.SortFunc(trades, func (a, b gamma.Trade) int {
		return cmp.Compare(a.Timestamp, b.Timestamp)
	})
	output := "time,price,size\n"
	for _, trade := range trades {
		timestamp := time.Unix(trade.Timestamp, 0)
		output += fmt.Sprintf("%s,%.3f,%.2f\n", commons.GetTimeString(timestamp), trade.Price, trade.Size)
	}
	commons.WriteFile(path, output)
	log.Printf("Wrote file %s (%d records)", path, len(trades))
}