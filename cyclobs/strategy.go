package cyclobs

import (
	"time"
)

type decayStrategy struct {
	tags []string
	triggerPriceMin float64
	triggerPriceMax float64
	positionSize float64
	holdingTime int
	priceRangeCheck bool
}

type StrategyResult struct {
	Tag string `json:"tag"`
	Parameter string `json:"parameter"`
	SharpeRatio float64 `json:"sharpeRatio"`
}

type thresholdStrategy struct {
	tags []string
	threshold float64
	greaterThan bool
	positionSize float64
	side backtestPositionSide
}

type jumpStrategy struct {
	includeTags []string
	excludeTags []string
	threshold1 float64
	threshold2 float64
	threshold3 float64
	stopLoss bool
	positionSize float64
	holdingTime int
	previousPrices map[string]priceSample
}

type mentionStrategy struct {
	threshold1 float64
	threshold2 float64
	minSamples int
	positionSize float64
	sampleCounts map[string]int
}

type priceSample struct {
	timestamp time.Time
	price float64
}

func (s *decayStrategy) next(backtest *backtestData) {
	markets := backtest.getMarkets(s.tags)
	for _, market := range markets {
		price, exists := backtest.getPriceErr(market.Slug)
		if !exists {
			continue
		}
		if price >= s.triggerPriceMin && price < s.triggerPriceMax {
			exists := containsFunc(backtest.positions, func (p backtestPosition) bool {
				return p.slug == market.Slug
			})
			if exists {
				continue
			}
			size := s.positionSize / price
			_ = backtest.openPosition(market.Slug, sideNo, size)
		}
	}
	for _, position := range backtest.positions {
		expired := backtest.now.Sub(position.timestamp) >= time.Duration(s.holdingTime) * time.Hour
		if s.priceRangeCheck {
			price := backtest.getPrice(position.slug)
			if expired || price < 0.4 {
				backtest.closePositions(position.slug)
			}
		} else {
			if expired {
				backtest.closePositions(position.slug)
			}
		}
	}
}

func (s *thresholdStrategy) next(backtest *backtestData) {
	markets := backtest.getMarkets(s.tags)
	for _, market := range markets {
		exists := containsFunc(backtest.positions, func (p backtestPosition) bool {
			return p.slug == market.Slug
		})
		if exists {
			continue
		}
		price, exists := backtest.getPriceErr(market.Slug)
		if !exists {
			continue
		}
		if (!s.greaterThan && price <= s.threshold) || (s.greaterThan && price >= s.threshold) {
			_ = backtest.openPosition(market.Slug, s.side, s.positionSize)
		}
	}
}

func (s *jumpStrategy) next(backtest *backtestData) {
	markets := backtest.getMarkets(s.includeTags)
	for _, market := range markets {
		excluded := false
		for _, tag := range market.Tags {
			if contains(s.excludeTags, tag) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		exists := containsFunc(backtest.positions, func (p backtestPosition) bool {
			return p.slug == market.Slug
		})
		if exists {
			continue
		}
		price, exists := backtest.getPriceErr(market.Slug)
		if !exists {
			continue
		}
		previous, exists := s.previousPrices[market.Slug]
		age := backtest.now.Sub(previous.timestamp)
		if exists && age <= time.Duration(1) * time.Hour && previous.price <= s.threshold1 && price >= s.threshold2 && price < s.threshold3 {
			_ = backtest.openPosition(market.Slug, sideNo, s.positionSize)
		}
		s.previousPrices[market.Slug] = priceSample{
			timestamp: backtest.now,
			price: price,
		}
	}
	for _, position := range backtest.positions {
		expired := backtest.now.Sub(position.timestamp) >= time.Duration(s.holdingTime) * time.Hour
		stopLoss := false
		if s.stopLoss {
			price, exists := backtest.getPriceErr(position.slug)
			if exists {
				stopLoss = price > s.threshold3
			}
		}
		if expired || stopLoss {
			backtest.closePositions(position.slug)
		}
	}
}

func (s *mentionStrategy) next(backtest *backtestData) {
	tags := []string{
		"mention-markets",
	}
	markets := backtest.getMarkets(tags)
	for _, market := range markets {
		slug := market.Slug
		exists := containsFunc(backtest.positions, func (p backtestPosition) bool {
			return p.slug == slug
		})
		if exists {
			continue
		}
		price, exists := backtest.getPriceErr(slug)
		if !exists {
			continue
		}
		s.sampleCounts[slug]++
		if s.sampleCounts[slug] == s.minSamples {
			if price >= s.threshold1 && price < s.threshold2 {
				_ = backtest.openPosition(market.Slug, sideYes, s.positionSize)
			}
		}
	}	
}