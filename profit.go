package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"slices"

	"github.com/encratite/commons"
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
	profits []activityProfit
	lastRow bool
	total float64
	returns []float64
	wins int
}

type activityPosition struct {
	outcomeIndex int
	price float64
	size float64
}

func analyzeProfits() {
	loadConfiguration()
	activities := getAllActivities()
	ignorePatterns := []*regexp.Regexp{}
	for _, group := range profitConfiguration.Ignore {
		for _, filter := range group.Filters {
			pattern := regexp.MustCompile(filter)
			ignorePatterns = append(ignorePatterns, pattern)
		}
	}
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
		}
		categories = append(categories, category)
	}
	profits := []activityProfit{}
	for _, activity := range activities {
		slug := activity.Slug
		ignored := false
		for _, pattern := range ignorePatterns {
			if pattern.MatchString(slug) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}
		i := slices.IndexFunc(profits, func (p activityProfit) bool {
			return p.slug == slug
		})
		profitExists := i >= 0
		isBuy := activity.Type == activityTypeTrade && activity.Side == activitySideBuy
		isSell := activity.Type == activityTypeTrade && activity.Side == activitySideSell
		isRedeem := activity.Type == activityTypeRedeem
		if isBuy {
			price := activity.USDCSize / activity.Size
			position := activityPosition{
				outcomeIndex: activity.OutcomeIndex,
				price: price,
				size: activity.Size,
			}
			if !profitExists {
				var category *activityCategory
				for i := range categories {
					c := &categories[i]
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
				if category == nil {
					fmt.Printf("Warning: unable to find a matching category for \"%s\"\n", slug)
					continue
				}
				profit := activityProfit{
					slug: slug,
					category: category,
					positions: []activityPosition{},
					sellPrice: 0.0,
					sold: false,
				}
				profit.positions = append(profit.positions, position)
				profits = append(profits, profit)
			} else {
				profit := &profits[i]
				profit.positions = append(profit.positions, position)
			}
		} else if isSell && profitExists {
			profit := &profits[i]
			profit.sellPrice += activity.USDCSize
			profit.sold = true
		} else if isRedeem && profitExists {
			profit := &profits[i]
			if profit.sold && profit.sellPrice > 0.0 {
				// fmt.Printf("Warning: redeem after redeem/sell for %s\n", activity.Slug)
				continue
			}
			market, err := getMarket(activity.Slug)
			if err != nil {
				return
			}
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
			sellPrice := 0.0
			for _, position := range profit.positions {
				if position.outcomeIndex == outcomeIndex {
					sellPrice += position.size
				}
			}
			if printResolveMessages {
				fmt.Printf("Resolved market %s to outcome %d for %s\n", activity.Slug, outcomeIndex, commons.FormatMoney(sellPrice))
			}
			profit.sellPrice += sellPrice
			profit.sold = true
		}
	}
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
	categories = append(categories, allCategory)
	header := []string{
		"Category",
		"Total PnL",
		"Return per Bet",
		"Risk-Adjusted Return",
		"Hit Rate",
		"Number of Bets",
	}
	rows := [][]string{}
	for _, category := range categories {
		category.processProfits()
		riskAdjustedString := "-"
		if len(category.returns) >= 2 {
			riskAdjusted := stat.Mean(category.returns, nil) / stat.StdDev(category.returns, nil)
			riskAdjustedString = fmt.Sprintf("%.2f", riskAdjusted)
		}
		percentage := percent * stat.Mean(category.returns, nil)
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
			commons.FormatMoney(category.total),
			fmt.Sprintf("%+.2f%%", percentage),
			riskAdjustedString,
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
	alignmentConfig := tablewriter.WithAlignment(alignments)
	table := tablewriter.NewTable(os.Stdout, tableConfig, alignmentConfig)
	table.Header(header)
	table.Bulk(rows)
	table.Render()
	allCategory.processProfits()
	p := allCategory.getPValue()
	fmt.Printf("\np-value: %.2f", p)
}

func getAllActivities() []Activity {
	output := []Activity{}
	for offset := 0;; offset += activityAPILimit {
		activities, err := getActivities(configuration.Credentials.ProxyAddress, offset)
		if err != nil {
			log.Fatalf("Failed to download activites: %v", err)
		}
		output = append(output, activities...)
		if len(activities) < activityAPILimit {
			break
		}
	}
	return output
}

func (c *activityCategory) processProfits() {
	c.total = 0.0
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
		c.total += delta
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