package main

import (
	"fmt"
	"log"
	"time"

	"github.com/encratite/commons"
	"github.com/encratite/gamma"
	"github.com/fatih/color"
	"github.com/gammazero/deque"
	"github.com/shopspring/decimal"
)

type jumpTradingSystem struct {
	threshold1 decimal.Decimal
	threshold2 decimal.Decimal
	threshold3 decimal.Decimal
	includeTags []string
	excludeTags []string
	subscriptions map[string]jumpSubscription
}

type jumpSubscription struct {
	market gamma.Market
	yesID string
	prices deque.Deque[jumpPriceEvent]
	triggered bool
	spread *decimal.Decimal
}

type jumpPriceEvent struct {
	timestamp time.Time
	price decimal.Decimal
}

func runJumpSystem() {
	loadConfiguration()
	config := configuration.Jump
	system := jumpTradingSystem{
		threshold1: config.Threshold1.Decimal,
		threshold2: config.Threshold2.Decimal,
		threshold3: config.Threshold3.Decimal,
		includeTags: config.IncludeTags,
		excludeTags: config.ExcludeTags,
		subscriptions: map[string]jumpSubscription{},
	}
	system.run()
}

func (s *jumpTradingSystem) run() {
	sleep := func () {
		time.Sleep(time.Duration(reconnectDelay) * time.Second)
	}
	for {
		markets := s.getMarkets()
		if markets == nil {
			sleep()
			continue
		}
		assetIDs := []string{}
		for _, market := range markets {
			yesID, err := getCLOBTokenID(market, true)
			if err != nil {
				continue
			}
			s.subscriptions[market.ConditionID] = jumpSubscription{
				market: market,
				yesID: yesID,
				prices: deque.Deque[jumpPriceEvent]{},
				triggered: false,
				spread: nil,
			}
			assetIDs = append(assetIDs, yesID)
		}
		log.Printf("Subscribed to %d markets", len(assetIDs))
		err := gamma.SubscribeToMarkets(assetIDs, s.onBookMessage)
		if err != nil {
			log.Printf("Subscription error: %v", err)
		}
		sleep()
	}
}

func (s *jumpTradingSystem) getMarkets() []gamma.Market {
	markets := []gamma.Market{}
	if len(s.includeTags) > 0 {
		for _, tagSlug := range s.includeTags {
			err := s.addMarkets(&tagSlug, &markets)
			if err != nil {
				break
			}
		}
	} else {
		s.addMarkets(nil, &markets)
	}
	return markets
}

func (s *jumpTradingSystem) addMarkets(tagSlug *string, markets *[]gamma.Market) error {
	const negRisk = false
	events, err := gamma.GetEvents(tagSlug)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Closed || !event.Active || event.NegRisk != negRisk {
			continue
		}
		include := true
		for _, tag := range event.Tags {
			if commons.Contains(s.excludeTags, tag.Slug) {
				include = false
				break
			}
		}
		if include {
			for _, market := range event.Markets {
				if market.Volume != "0" {
					*markets = append(*markets, market)
				}
			}
		}
	}
	return nil
}

func (s *jumpTradingSystem) onBookMessage(message gamma.BookMessage) bool {
	key := message.Market
	subscription, exists := s.subscriptions[key]
	if !exists {
		log.Printf("Warning: received a message for an unknown subscription for market %s", message.Market)
		return true
	}
	switch message.EventType {
	case gamma.PriceChangeEvent:
		s.onPriceChange(message, &subscription)
	case gamma.LastTradePriceEvent:
		s.onLastTradePrice(message, &subscription)
	}
	s.subscriptions[key] = subscription
	return true
}

func (s *jumpTradingSystem) onPriceChange(message gamma.BookMessage, subscription *jumpSubscription) {
	for _, change := range message.PriceChanges {
		if change.AssetID == subscription.yesID {
			bestAsk, err := decimal.NewFromString(change.BestAsk)
			if err != nil {
				log.Printf("Failed to parse best ask: %s", change.BestAsk)
				return
			}
			bestBid, err := decimal.NewFromString(change.BestBid)
			if err != nil {
				log.Printf("Failed to parse best bid: %s", change.BestBid)
				return
			}
			spread := bestAsk.Sub(bestBid)
			subscription.spread = &spread
		}
	}
}

func (s *jumpTradingSystem) onLastTradePrice(message gamma.BookMessage, subscription *jumpSubscription) {
	price, _, err := getPriceSize(message.Price, message.Size)
	if err != nil {
		return
	}
	addPrice(price, &subscription.prices)
	firstPrice := subscription.prices.Front().price
	if firstPrice.LessThanOrEqual(s.threshold1) && price.GreaterThanOrEqual(s.threshold2) && price.LessThan(s.threshold3) {
		green := color.New(color.FgGreen).SprintfFunc()
		format := "slug = %s, firstPrice = %s, price = %s, spread = %s"
		message := fmt.Sprintf(format, subscription.market.Slug, firstPrice, price, subscription.spread)
		if !subscription.triggered {
			log.Printf("Triggered: %s", green(message))
			subscription.triggered = true
			beep()
		} else {
			log.Printf("In range: %s", message)
		}
	}
}

func addPrice(price decimal.Decimal, prices *deque.Deque[jumpPriceEvent]) {
	event := jumpPriceEvent{
		timestamp: time.Now(),
		price: price,
	}
	prices.PushBack(event)
	now := time.Now()
	for prices.Len() > 0 {
		price := prices.Front()
		age := now.Sub(price.timestamp)
		if age > time.Hour {
			prices.PopFront()
		} else {
			break
		}
	}
}