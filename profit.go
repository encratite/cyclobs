package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"

	"github.com/encratite/commons"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"gonum.org/v1/gonum/stat"
)

const (
	activityTypeRedeem = "REDEEM"
	activityTypeTrade = "TRADE"
	activitySideBuy = "BUY"
	activitySideSell = "SELL"
)

type activityProfit struct {
	slug string
	category *activityCategory
	buyPrice float64
	sellPrice float64
	sold bool
}

type activityCategory struct {
	name string
	patterns []*regexp.Regexp
	profits []activityProfit
	lastRow bool
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
				}
				if category == nil {
					fmt.Printf("Warning: unable to find a matching category for \"%s\"\n", slug)
					continue
				}
				profit := activityProfit{
					slug: slug,
					category: category,
					buyPrice: activity.USDCSize,
					sellPrice: 0.0,
					sold: false,
				}
				profits = append(profits, profit)
			} else {
				profit := &profits[i]
				profit.buyPrice += activity.USDCSize
			}
		} else if (isSell || isRedeem) && profitExists {
			profit := &profits[i]
			profit.sellPrice += activity.USDCSize
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
		total := 0.0
		returns := []float64{}
		wins := 0
		for _, profit := range category.profits {
			if !profit.sold {
				continue
			}
			delta := profit.sellPrice - profit.buyPrice
			total += delta
			r := profit.sellPrice / profit.buyPrice - 1.0
			if r > 0.0 {
				wins++
			}
			returns = append(returns, r)
		}
		riskAdjustedString := "-"
		if len(returns) >= 2 {
			riskAdjusted := stat.Mean(returns, nil) / stat.StdDev(returns, nil)
			riskAdjustedString = fmt.Sprintf("%.2f", riskAdjusted)
		}
		percentage := percent * stat.Mean(returns, nil)
		hitRate := percent * float64(wins) / float64(len(returns))
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
			commons.FormatMoney(total),
			fmt.Sprintf("%+.2f%%", percentage),
			riskAdjustedString,
			fmt.Sprintf("%.1f%%", hitRate),
			commons.IntToString(len(returns)),
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