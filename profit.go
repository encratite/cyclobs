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
	"github.com/fatih/color"
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
	activityTypeMerge = "MERGE"
	activityTypeReward = "REWARD"
	printResolveMessages = false
	printPValue = false
	activityDays = 7
	outcomeIndexYes = 0
	outcomeIndexNo = 1
)

type activityMarket struct {
	slug string
	category *activityCategory
	positions []activityPosition
	buyPrice float64
	sellPrice float64
	redeemed bool
	sold bool
}

type activityCategory struct {
	name string
	patterns []*regexp.Regexp
	after *time.Time
	markets []activityMarket
	lastRow bool
	totalBuy float64
	totalProfit float64
	returns []float64
	wins int
	disabled bool
}

type activityPosition struct {
	outcomeIndex int
	price float64
	size float64
	sold float64
	removed float64
}

func analyzeProfits(start, end *time.Time) {
	loadConfiguration()
	activities := getAllActivities(start, end)
	ignorePatterns, bypassPatterns := getIgnorePatterns()
	categories := getCategories()
	markets := []activityMarket{}
	processActivities(ignorePatterns, bypassPatterns, activities, &categories, &markets)
	if profitConfiguration.Live {
		processPositions(&categories, &markets)
	}
	allCategory := getAllCategory(markets)
	for i := range categories {
		category := &categories[i]
		if !category.disabled {
			category.processProfits()
		}
	}
	allCategory.processProfits()
	if profitConfiguration.Detailed {
		printCategoriesDetailed(categories)
	}
	printCategories(categories, allCategory)
}

func getAllActivities(start, end *time.Time) []gamma.Activity {
	output := []gamma.Activity{}
	currentEnd := time.Now().UTC()
	if end != nil {
		currentEnd = *end
	}
	running := true
	for running {
		currentStart := currentEnd.Add(time.Duration(- activityDays * hoursPerDay) * time.Hour)
		if start != nil && currentStart.Before(*start) {
			running = false
			currentStart = *start
		}
		// log.Printf("Activities: currentStart = %s, currentEnd = %s", commons.GetDateString(currentStart), commons.GetDateString(currentEnd))
		activities, err := gamma.GetActivities(configuration.Credentials.ProxyAddress, 0, currentStart, currentEnd)
		if err != nil {
			log.Fatalf("Failed to download activites: %v", err)
		}
		output = append(output, activities...)
		if len(activities) == gamma.ActivityAPILimit {
			log.Fatalf("Too many activities, decrease activityDays")
		}
		if len(activities) == 0 && start == nil {
			break
		}
		currentEnd = currentStart
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
			markets: []activityMarket{},
			lastRow: false,
			totalBuy: categoryData.Bet,
			totalProfit: categoryData.Profit,
			disabled: categoryData.Disabled,
		}
		if categoryData.After != nil {
			category.after = &categoryData.After.Time
		}
		categories = append(categories, category)
	}
	return categories
}

func processActivities(
	ignorePatterns []*regexp.Regexp,
	bypassPatterns []*regexp.Regexp,
	activities []gamma.Activity,
	categories *[]activityCategory,
	markets *[]activityMarket,
) {
	for _, activity := range activities {
		timestamp := time.Unix(activity.Timestamp, 0).UTC()
		if activity.Type == activityTypeReward {
			continue
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
		index := slices.IndexFunc(*markets, func (p activityMarket) bool {
			return p.slug == slug
		})
		profitExists := index >= 0
		isBuy := activity.Type == activityTypeTrade && activity.Side == activitySideBuy
		isSell := activity.Type == activityTypeTrade && activity.Side == activitySideSell
		isRedeem := activity.Type == activityTypeRedeem
		isMerge := activity.Type == activityTypeMerge
		if isBuy {
			processBuy(activity, timestamp, slug, index, profitExists, categories, markets)
		} else if isSell && profitExists {
			market := &(*markets)[index]
			remaining := activity.Size
			for i := range market.positions {
				position := &market.positions[i]
				if position.outcomeIndex == activity.OutcomeIndex {
					available := position.size - position.removed
					sold := min(remaining, available)
					position.removed += sold
					remaining -= sold
				}
			}
			market.sellPrice += activity.USDCSize
			market.sold = true
		} else if isRedeem && profitExists {
			market := &(*markets)[index]
			if market.redeemed {
				continue
			}
			slug := activity.Slug
			for _, rename := range profitConfiguration.RenamedSlugs {
				if rename.Old == slug {
					slug = rename.New
					break
				}
			}
			marketData, err := gamma.GetMarket(slug)
			if err != nil {
				event, err := gamma.GetEventBySlug(activity.EventSlug)
				if err != nil {
					fmt.Printf("Failed to use event fallback: %v\n", err)
					continue
				}
				match := false
				for _, m := range event.Markets {
					if len(activity.Slug) < len(m.Slug) && activity.Slug == m.Slug[0:len(activity.Slug)] {
						marketData = m
						match = true
						break
					}
				}
				if !match {
					fmt.Printf("Failed to find matching fallback for %s\n", activity.Slug)
					continue
				}
			}
			sellPrice := 0.0
			draw := isDraw(marketData)
			if !draw {
				outcome := getMarketOutcome(marketData)
				if outcome == nil {
					fmt.Printf("Warning: no outcome for market %s\n", marketData.Slug)
					continue
				}
				var outcomeIndex int
				if *outcome {
					outcomeIndex = outcomeIndexYes
				} else {
					outcomeIndex = outcomeIndexNo
				}
				for _, position := range market.positions {
					if position.outcomeIndex == outcomeIndex {
						sellPrice += position.size - position.removed
					}
				}
				if printResolveMessages {
					fmt.Printf("Resolved market %s to outcome %d for %s\n", activity.Slug, outcomeIndex, commons.FormatMoney(sellPrice))
				}
			} else {
				for _, position := range market.positions {
					sellPrice += 0.5 * (position.size - position.removed)
				}
			}
			market.sellPrice += sellPrice
			market.redeemed = true
		} else if isMerge && profitExists {
			market := &(*markets)[index]
			remainingYes := activity.Size
			remainingNo := activity.Size
			for i := range market.positions {
				position := &market.positions[i]
				processMerge := func (outcomeIndex int, remaining *float64) {
					if position.outcomeIndex == outcomeIndex {
						available := position.size - position.removed
						merged := min(*remaining, available)
						position.removed += merged
						*remaining -= merged
					}
				}
				processMerge(outcomeIndexYes, &remainingYes)
				processMerge(outcomeIndexNo, &remainingNo)
			}
			market.sellPrice += activity.USDCSize
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
	markets *[]activityMarket,
) {
	price := activity.USDCSize / activity.Size
	position := activityPosition{
		outcomeIndex: activity.OutcomeIndex,
		price: price,
		size: activity.Size,
		removed: 0.0,
	}
	if !profitExists {
		category := getMatchingCategory(timestamp, slug, categories)
		if category == nil {
			return
		}
		profit := activityMarket{
			slug: slug,
			category: category,
			positions: []activityPosition{},
			sellPrice: 0.0,
			redeemed: false,
		}
		profit.positions = append(profit.positions, position)
		*markets = append(*markets, profit)
	} else {
		market := &(*markets)[index]
		market.positions = append(market.positions, position)
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

func processPositions(categories *[]activityCategory, markets *[]activityMarket) {
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
		if category == nil || category.disabled {
			continue
		}
		buyPrice := position.InitialValue
		sellValue := position.Size * position.CurPrice
		profitPosition := activityPosition{
			outcomeIndex: 0,
			price: position.AvgPrice,
			size: position.Size,
		}
		profit := activityMarket{
			slug: position.Slug,
			category: category,
			positions: []activityPosition{profitPosition},
			buyPrice: buyPrice,
			sellPrice: sellValue,
			redeemed: false,
			sold: true,
		}
		*markets = append(*markets, profit)
	}
}

func getAllCategory(profits []activityMarket) activityCategory {
	allCategory := activityCategory{
		name: "All",
		patterns: nil,
		markets: []activityMarket{},
		lastRow: true,
	}
	for _, profit := range profits {
		category := profit.category
		if !category.disabled {
			category.markets = append(category.markets, profit)
			allCategory.markets = append(allCategory.markets, profit)
		}
	}
	return allCategory
}

func (c *activityCategory) processProfits() {
	c.returns = []float64{}
	c.wins = 0
	for i := range c.markets {
		market := &c.markets[i]
		buyPrice := 0.0
		for _, position := range market.positions {
			buyPrice += position.size * position.price
		}
		market.buyPrice = buyPrice
		if !market.redeemed && market.sellPrice == 0.0 {
			continue
		}
		delta := market.sellPrice - buyPrice
		c.totalBuy += buyPrice
		c.totalProfit += delta
		r := market.sellPrice / buyPrice - 1.0
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
		totalProfitString := formatNumeric(category.totalProfit, commons.FormatProfit)
		totalReturnString := formatNumeric(totalReturn, func (value float64) string {
			return fmt.Sprintf("%+.2f%%", value)
		})
		row := []string{
			category.name,
			totalProfitString,
			commons.FormatMoney(category.totalBuy),
			totalReturnString,
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
	if printPValue {
		p := allCategory.getPValue()
		fmt.Printf("\np-value: %.3f\n\n", p)
	}
	fmt.Printf("\n")
}

func printCategoriesDetailed(categories []activityCategory) {
	for _, category := range categories {
		fmt.Printf("\n%s:\n", category.name)
		for _, market := range category.markets {
			if market.redeemed || market.sold {
				delta := market.sellPrice - market.buyPrice
				profitString := formatNumeric(delta, commons.FormatProfit)
				fmt.Printf("\t%s: %s\n", market.slug, profitString)
			}
		}
	}
}

func formatNumeric(value float64, format func (float64) string) string {
	formatted := format(value)
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	if value > 0 {
		formatted = green(formatted)
	}
	if value < 0 {
		formatted = red(formatted)
	}
	return formatted
}