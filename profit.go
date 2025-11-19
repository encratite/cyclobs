package main

import (
	"cmp"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"slices"
	"time"

	"github.com/encratite/commons"
	"github.com/encratite/gamma"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/gonum/stat/distuv"
)

const (
	activityTypeRedeem = "REDEEM"
	activityTypeTrade = "TRADE"
	activitySideBuy = "BUY"
	activitySideSell = "SELL"
	printResolveMessages = false
	activityDays = 7
	printPValue = false
)

type activityProfit struct {
	slug string
	category *activityCategory
	positions []activityPosition
	sellPrice float64
	sold bool
}

type activityCategory struct {
	name string
	patterns []*regexp.Regexp
	after *time.Time
	profits []activityProfit
	lastRow bool
	totalBuy float64
	totalProfit float64
	returns []float64
	wins int
}

type activityPosition struct {
	outcomeIndex int
	price float64
	size float64
}

func analyzeProfits(dateString string) {
	loadConfiguration()
	var date time.Time
	hasDate := false
	date, err := commons.ParseTime(dateString)
	if err == nil {
		hasDate = true
	}
	activities := getAllActivities()
	ignorePatterns, bypassPatterns := getIgnorePatterns()
	categories := getCategories()
	profits := []activityProfit{}
	processActivities(date, hasDate, ignorePatterns, bypassPatterns, activities, &categories, &profits)
	if profitConfiguration.Live {
		processPositions(&categories, &profits)
	}
	allCategory := getAllCategory(profits)
	printCategories(categories, allCategory)
}

func getAllActivities() []gamma.Activity {
	output := []gamma.Activity{}
	end := time.Now().UTC()
	for {
		start := end.Add(time.Duration(- activityDays * hoursPerDay) * time.Hour)
		activities, err := gamma.GetActivities(configuration.Credentials.ProxyAddress, 0, start, end)
		if err != nil {
			log.Fatalf("Failed to download activites: %v", err)
		}
		output = append(output, activities...)
		if len(activities) == gamma.ActivityAPILimit {
			log.Fatalf("Too many activities, decrease activityDays")
		}
		if len(activities) == 0 {
			break
		}
		end = start
	}
	slices.SortFunc(output, func (a, b gamma.Activity) int {
		return cmp.Compare(a.Timestamp, b.Timestamp)
	})
	return output
}

func getIgnorePatterns() ([]*regexp.Regexp, []*regexp.Regexp) {
	ignorePatterns := []*regexp.Regexp{}
	bypassPatterns := []*regexp.Regexp{}
	for _, group := range profitConfiguration.Ignore {
		for _, filter := range group.Filters {
			pattern := regexp.MustCompile(filter)
			ignorePatterns = append(ignorePatterns, pattern)
		}
		for _, filter := range group.Bypass {
			pattern := regexp.MustCompile(filter)
			bypassPatterns = append(bypassPatterns, pattern)
		}
	}
	return ignorePatterns, bypassPatterns
}

func getCategories() []activityCategory {
	categories := []activityCategory{}
	for _, categoryData := range profitConfiguration.Categories {
		patterns := []*regexp.Regexp{}
		for _, filter := range categoryData.Filters {
			pattern := regexp.MustCompile(filter)
			patterns = append(patterns, pattern)
		}
		category := activityCategory{
			name: categoryData.Name,
			patterns: patterns,
			profits: []activityProfit{},
			lastRow: false,
			totalBuy: categoryData.Bet,
			totalProfit: categoryData.Profit,
		}
		if categoryData.After != nil {
			category.after = &categoryData.After.Time
		}
		categories = append(categories, category)
	}
	return categories
}

func processActivities(
	date time.Time,
	hasDate bool,
	ignorePatterns []*regexp.Regexp,
	bypassPatterns []*regexp.Regexp,
	activities []gamma.Activity,
	categories *[]activityCategory,
	profits *[]activityProfit,
) {
	for _, activity := range activities {
		timestamp := time.Unix(activity.Timestamp, 0).UTC()
		if hasDate {
			if timestamp.Before(date) {
				continue
			}
		}
		slug := activity.Slug
		ignored := false
		for _, pattern := range ignorePatterns {
			if pattern.MatchString(slug) {
				ignored = true
				break
			}
		}
		bypass := false
		for _, pattern := range bypassPatterns {
			if pattern.MatchString(slug) {
				bypass = true
				break
			}
		}
		if ignored && !bypass {
			continue
		}
		index := slices.IndexFunc(*profits, func (p activityProfit) bool {
			return p.slug == slug
		})
		profitExists := index >= 0
		isBuy := activity.Type == activityTypeTrade && activity.Side == activitySideBuy
		isSell := activity.Type == activityTypeTrade && activity.Side == activitySideSell
		isRedeem := activity.Type == activityTypeRedeem
		if isBuy {
			processBuy(activity, timestamp, slug, index, profitExists, categories, profits)
		} else if isSell && profitExists {
			profit := &(*profits)[index]
			profit.sellPrice += activity.USDCSize
			profit.sold = true
		} else if isRedeem && profitExists {
			profit := &(*profits)[index]
			if profit.sold && profit.sellPrice > 0.0 {
				// fmt.Printf("Warning: redeem after redeem/sell for %s\n", activity.Slug)
				continue
			}
			market, err := gamma.GetMarket(activity.Slug)
			if err != nil {
				return
			}
			sellPrice := 0.0
			draw := isDraw(market)
			if !draw {
				outcome := getMarketOutcome(market)
				if outcome == nil {
					fmt.Printf("Warning: no outcome for market %s\n", market.Slug)
					continue
				}
				var outcomeIndex int
				if *outcome {
					outcomeIndex = 0
				} else {
					outcomeIndex = 1
				}
				for _, position := range profit.positions {
					if position.outcomeIndex == outcomeIndex {
						sellPrice += position.size
					}
				}
				if printResolveMessages {
					fmt.Printf("Resolved market %s to outcome %d for %s\n", activity.Slug, outcomeIndex, commons.FormatMoney(sellPrice))
				}
			} else {
				for _, position := range profit.positions {
					sellPrice += 0.5 * position.size
				}
			}
			profit.sellPrice += sellPrice
			profit.sold = true
		}
	}
}

func processBuy(
	activity gamma.Activity,
	timestamp time.Time,
	slug string,
	index int,
	profitExists bool,
	categories *[]activityCategory,
	profits *[]activityProfit,
) {
	price := activity.USDCSize / activity.Size
	position := activityPosition{
		outcomeIndex: activity.OutcomeIndex,
		price: price,
		size: activity.Size,
	}
	if !profitExists {
		category := getMatchingCategory(timestamp, slug, categories)
		if category == nil {
			return
		}
		profit := activityProfit{
			slug: slug,
			category: category,
			positions: []activityPosition{},
			sellPrice: 0.0,
			sold: false,
		}
		profit.positions = append(profit.positions, position)
		*profits = append(*profits, profit)
	} else {
		profit := &(*profits)[index]
		profit.positions = append(profit.positions, position)
	}
}

func getMatchingCategory(timestamp time.Time, slug string, categories *[]activityCategory) *activityCategory {
	var category *activityCategory
	outOfRange := false
	for i := range *categories {
		c := &(*categories)[i]
		if c.after != nil && timestamp.Before(*c.after) {
			outOfRange = true
			break
		}
		for _, pattern := range c.patterns {
			if pattern.MatchString(slug) {
				category = c
				break
			}
		}
		if category != nil {
			break
		}
	}
	if outOfRange {
		return nil
	}
	if category == nil {
		fmt.Printf("Warning: unable to find a matching category for \"%s\"\n", slug)
		return nil
	}
	return category
}

func processPositions(categories *[]activityCategory, profits *[]activityProfit) {
	positions, err := gamma.GetPositions(configuration.Credentials.ProxyAddress)
	if err != nil {
		log.Fatalf("Failed to get positions: %v", err)
	}
	for _, position := range positions {
		// Using an unadjusted end date as the start date for range checks doesn't really make any sense
		date, err := commons.ParseTime(position.EndDate)
		if err != nil {
			fmt.Printf("Warning: unable to parse end date of position %s\n", position.Slug)
			continue
		}
		category := getMatchingCategory(date, position.Slug, categories)
		if category == nil {
			continue
		}
		sellValue := position.Size * position.CurPrice
		profitPosition := activityPosition{
			outcomeIndex: 0,
			price: position.AvgPrice,
			size: position.Size,
		}
		profit := activityProfit{
			slug: position.Slug,
			category: category,
			positions: []activityPosition{profitPosition},
			sellPrice: sellValue,
			sold: true,
		}
		*profits = append(*profits, profit)
	}
}

func getAllCategory(profits []activityProfit) activityCategory {
	allCategory := activityCategory{
		name: "All",
		patterns: nil,
		profits: []activityProfit{},
		lastRow: true,
	}
	for _, profit := range profits {
		category := profit.category
		category.profits = append(category.profits, profit)
		allCategory.profits = append(allCategory.profits, profit)
	}
	return allCategory
}

func (c *activityCategory) processProfits() {
	c.returns = []float64{}
	c.wins = 0
	for _, profit := range c.profits {
		if !profit.sold {
			continue
		}
		buyPrice := 0.0
		for _, position := range profit.positions {
			buyPrice += position.size * position.price
		}
		delta := profit.sellPrice - buyPrice
		c.totalBuy += buyPrice
		c.totalProfit += delta
		r := profit.sellPrice / buyPrice - 1.0
		if r > 0.0 {
			c.wins++
		}
		c.returns = append(c.returns, r)
	}
}

func (c *activityCategory) getPValue() float64 {
	returns := c.returns
	mean := stat.Mean(returns, nil)
	stdDev := stat.StdDev(returns, nil)
	n := float64(len(returns))
	degrees := n - 1.0
	distribution := distuv.StudentsT{
		Mu: 0,
		Sigma: 1,
		Nu: degrees,
	}
	const randomMean = 0.0
	Z := mean - randomMean
	s := stdDev / math.Sqrt(n)
	t := Z / s
	p := 1 - distribution.CDF(t)
	return p
}

func isDraw(market gamma.Market) bool {
	return market.OutcomePrices == "[\"0.5\", \"0.5\"]"
}

func printCategories(categories []activityCategory, allCategory activityCategory) {
	categories = append(categories, allCategory)
	header := []string{
		"Category",
		"Total PnL",
		"Volume",
		"Total Return",
		"Hit Rate",
		"Markets Traded",
	}
	rows := [][]string{}
	for _, category := range categories {
		category.processProfits()
		if len(category.returns) == 0 {
			continue
		}
		totalReturn := percent * category.totalProfit / category.totalBuy
		hitRate := percent * float64(category.wins) / float64(len(category.returns))
		if category.lastRow {
			emptyRow := []string{
				"",
				"",
				"",
				"",
				"",
				"",
			}
			rows = append(rows, emptyRow)
		}
		row := []string{
			category.name,
			commons.FormatMoney(category.totalProfit),
			commons.FormatMoney(category.totalBuy),
			fmt.Sprintf("%+.2f%%", totalReturn),
			fmt.Sprintf("%.1f%%", hitRate),
			commons.IntToString(len(category.returns)),
		}
		rows = append(rows, row)
	}
	alignments := []tw.Align{
		tw.AlignDefault,
		tw.AlignRight,
		tw.AlignRight,
		tw.AlignRight,
		tw.AlignRight,
		tw.AlignRight,
	}
	tableConfig := tablewriter.WithConfig(tablewriter.Config{
		Header: tw.CellConfig{
			Formatting: tw.CellFormatting{AutoFormat: tw.Off},
			Alignment: tw.CellAlignment{Global: tw.AlignLeft},
		}},
	)
	fmt.Printf("\n")
	alignmentConfig := tablewriter.WithAlignment(alignments)
	table := tablewriter.NewTable(os.Stdout, tableConfig, alignmentConfig)
	table.Header(header)
	table.Bulk(rows)
	table.Render()
	allCategory.processProfits()
	if printPValue {
		p := allCategory.getPValue()
		fmt.Printf("\np-value: %.3f\n\n", p)
	}
}