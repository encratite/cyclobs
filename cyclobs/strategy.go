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

type DecayStrategyResult struct {
	Tag string `json:"tag"`
	Parameter string `json:"parameter"`
	SharpeRatio float64 `json:"sharpeRatio"`
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
}