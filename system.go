package main

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/encratite/commons"
	"github.com/gammazero/deque"
	"github.com/polymarket/go-order-utils/pkg/model"
	"github.com/shopspring/decimal"
)

const (
	eventsLimit = 250
	reconnectDelay = 5
	bookEvent = "book"
	priceChangeEvent = "price_change"
	lastTradePriceEvent = "last_trade_price"
	debugPriceChange = false
	debugLastTradePrice = true
	debugOrderBook = false
	debugTrigger = true
	bookSidePrintLimit = 5
)

type tradingSystemMode int

const (
	systemDataMode tradingSystemMode = iota
	systemTriggerMode
)

type tradingSystem struct {
	mode tradingSystemMode
	markets []Market
	subscriptions map[string]marketSubscription
	database databaseClient
	triggers []triggerData
}

type marketSubscription struct {
	slug string
	conditionID string
	assetID string
	negRisk bool
	prices deque.Deque[priceEvent]
	bids *treemap.Map
	asks *treemap.Map
}

type priceEvent struct {
	timestamp time.Time
	price decimal.Decimal
	size decimal.Decimal
}

type triggerData struct {
	slug string
	assetID string
	size decimal.Decimal
	trigger Trigger
	triggered bool
}

func runMode(mode tradingSystemMode) {
	loadConfiguration()
	database := newDatabaseClient()
	system := tradingSystem{
		mode: mode,
		markets: []Market{},
		subscriptions: map[string]marketSubscription{},
		database: database,
		triggers: []triggerData{},
	}
	system.run()
}

func (s *tradingSystem) run() {
	defer s.database.close()
	s.interrupt()
	if s.mode == systemTriggerMode && *configuration.Trigger.Live {
		log.Printf("Warning: system is LIVE and has permission to post orders")
	}
	for {
		switch s.mode {
		case systemDataMode:
			s.runDataMode()
		case systemTriggerMode:
			s.runTriggerMode()
		default:
			log.Fatalf("Unknown system mode: %d", s.mode)
		}
		time.Sleep(time.Duration(reconnectDelay) * time.Second)
	}
}

func (s *tradingSystem) runDataMode() {
	markets, eventSlugMap, err := getEventMarkets()
	if err != nil {
		return
	}
	printMarketStats(markets)
	for _, eventSlug := range configuration.Data.Events {
		event, err := getEventBySlug(eventSlug)
		if err != nil {
			return
		}
		for _, market := range event.Markets {
			eventSlugMap[market.Slug] = event.Slug
			markets = append(markets, market)
		}
		log.Printf("Loaded %d additional markets from event %s", len(event.Markets), eventSlug)
	}
	s.markets = markets
	assetIDs := getAssetIDs(markets)
	s.database.insertMarkets(markets, assetIDs, eventSlugMap)
	log.Printf("Subscribed to %d markets", len(assetIDs))
	s.subscribe(assetIDs)
}

func (s *tradingSystem) runTriggerMode() {
	positions, err := getPositions()
	if err != nil {
		return
	}
	markets := []Market{}
	for _, position := range positions {
		exists := commons.ContainsFunc(markets, func (m Market) bool {
			return m.Slug == position.Slug
		})
		if !exists {
			market, err := getMarket(position.Slug)
			if err != nil {
				return
			}
			markets = append(markets, market)
		}
	}
	s.markets = markets
	assetIDs := []string{}
	for _, trigger := range configuration.Trigger.Triggers {
		slug := *trigger.Slug
		position, exists := commons.Find(positions, func (p Position) bool {
			return p.Slug == slug
		})
		assetID := position.Asset
		if !exists {
			log.Fatalf("Unable to find a position matching trigger slug \"%s\"", slug)
		}
		assetIDs = append(assetIDs, assetID)
		log.Printf("Subscribed to market \"%s\"", slug)
		data := triggerData{
			slug: slug,
			assetID: assetID,
			size: decimal.NewFromFloat(position.Size),
			trigger: trigger,
			triggered: false,
		}
		s.triggers = append(s.triggers, data)
	}
	s.subscribe(assetIDs)
}

func (s *tradingSystem) subscribe(assetIDs []string) {
	err := subscribeToMarkets(assetIDs, s.onBookMessage)
	if err != nil {
		log.Printf("Subscription error: %v", err)
	}
}

func (s *tradingSystem) interrupt() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		log.Println("Received interrupt signal, flushing buffer")
		s.database.flushBuffer()
		os.Exit(0)
	}()
}

func (s *tradingSystem) onBookMessage(message BookMessage) {
	subscription, exists := s.getSubscription(message)
	if !exists {
		return
	}
	switch message.EventType {
	case bookEvent:
		s.onBookEvent(message, &subscription)
	case priceChangeEvent:
		s.onPriceChange(message, &subscription)
	case lastTradePriceEvent:
		s.onLastTradePrice(message, &subscription)
	}
	if s.mode == systemDataMode || (s.mode == systemTriggerMode && *configuration.Trigger.RecordData) {
		s.database.insertBookMessage(message, subscription)
	}
	_ = subscription.validateOrderBook()
	s.subscriptions[message.Market] = subscription
}

func (s *tradingSystem) onBookEvent(message BookMessage, subscription *marketSubscription) {
	putPriceLevels(subscription.bids, message.Bids)
	putPriceLevels(subscription.asks, message.Asks)
	if debugOrderBook {
		subscription.printOrderBook()
	}
}

func (s *tradingSystem) onPriceChange(message BookMessage, subscription *marketSubscription) {
	for i, change := range message.PriceChanges {
		if debugPriceChange {
			format := "%s[%d]: slug = %s, conditionID = %s, assetID = %s, price = %s, size = %s, side = %s, best_bid = %s, best_ask = %s"
			log.Printf(format, priceChangeEvent, i, subscription.slug, subscription.conditionID, subscription.assetID, change.Price, change.Size, change.Side, change.BestBid, change.BestAsk)
		}
		price, size, err := getPriceSize(change.Price, change.Size)
		if err != nil {
			continue
		}
		var side, otherSide *treemap.Map
		var bid bool
		switch change.Side {
		case sideBuy:
			bid = true
			side = subscription.bids
			otherSide = subscription.asks
		case sideSell:
			bid = false
			side = subscription.asks
			otherSide = subscription.bids
		default:
			continue
		}
		if size.IsPositive() {
			side.Put(price, size)
			removeKeys := []decimal.Decimal{}
			it := otherSide.Iterator()
			if bid {
				for it.Next() {
					key := it.Key().(decimal.Decimal)
					if key.LessThanOrEqual(price) {
						removeKeys = append(removeKeys, key)
					}
				}
			} else {
				it.End()
				for it.Prev() {
					key := it.Key().(decimal.Decimal)
					if key.GreaterThanOrEqual(price) {
						removeKeys = append(removeKeys, key)
					}
				}
			}
			for _, key := range removeKeys {
				otherSide.Remove(key)
			}
		} else if size.IsZero() {
			side.Remove(price)
			otherSide.Remove(price)
		} else {
			log.Printf("Warning: negative price change")
			side.Remove(price)
			otherSide.Remove(price)
		}
	}
	if debugOrderBook {
		subscription.printOrderBook()
	}
}

func (s *tradingSystem) onLastTradePrice(message BookMessage, subscription *marketSubscription) {
	price, size, err := getPriceSize(message.Price, message.Size)
	if err != nil {
		return
	}
	event := priceEvent{
		timestamp: time.Now(),
		price: price,
		size: size,
	}
	if debugLastTradePrice {
		log.Printf("%s: slug = %s, price = %s, size = %s, side = %s", lastTradePriceEvent, subscription.slug, message.Price, message.Size, message.Side)
	}
	subscription.add(event)
	if s.mode == systemTriggerMode {
		s.processTrigger(price, message.Side, subscription)
	}
}

func (s *tradingSystem) processTrigger(price decimal.Decimal, side string, subscription *marketSubscription) {
	trigger, exists := commons.FindPointer(s.triggers, func (t triggerData) bool {
		return t.slug == subscription.slug
	})
	if trigger.triggered {
		if debugTrigger {
			log.Printf("Trigger for \"%s\" had already been triggered", trigger.slug)
		}
		return
	}
	if !exists {
		log.Printf("Warning: received a book message without a matching trigger: subscription.slug = %s", subscription.slug)
	}
	definition := trigger.trigger
	takeProfit := definition.TakeProfit
	sellPosition := func (limit decimal.Decimal) {
		err := postOrder(trigger.slug, trigger.assetID, model.SELL, trigger.size, limit, subscription.negRisk, 0)
		if err != nil {
			log.Printf("Failed to execute order: %v", err)
			return
		}
		trigger.triggered = true
	}
	if takeProfit != nil && price.GreaterThanOrEqual(takeProfit.Decimal) && side == sideBuy {
		log.Printf("Take profit has been triggered for \"%s\" at %s", trigger.slug, price)
		go sellPosition(definition.TakeProfitLimit.Decimal)
	} else if price.LessThanOrEqual(definition.StopLoss.Decimal) && side == sideSell {
		log.Printf("Stop-loss has been triggered for \"%s\" at %s", trigger.slug, price)
		go sellPosition(definition.StopLossLimit.Decimal)
	} else {
		if debugTrigger {
			format := "No action required: takeProfit = %s, takeProfitLimit = %s, stopLoss = %s, stopLossLimit = %s, size = %s, price = %s, side = %s"
			log.Printf(format, takeProfit, definition.TakeProfitLimit, definition.StopLoss, definition.StopLossLimit, trigger.size, price, side)
		}
	}
}

func (s *tradingSystem) getMarket(conditionID string) (Market, bool) {
	market, exists := commons.Find(s.markets, func (market Market) bool {
		return market.ConditionID == conditionID
	})
	if exists {
		return market, true
	} else {
		return Market{}, false
	}
}

func (s *tradingSystem) getSubscription(message BookMessage) (marketSubscription, bool) {
	conditionID := message.Market
	assetID := message.AssetID
	subscription, exists := s.subscriptions[conditionID]
	if !exists {
		if assetID == "" {
			log.Printf("Warning: invalid asset ID in subscription %s", conditionID)
		}
		market, exists := s.getMarket(conditionID)
		if !exists {
			log.Printf("Warning: unable to find matching market with condition ID %s", conditionID)
			return marketSubscription{}, false
		}
		subscription = marketSubscription{
			slug: market.Slug,
			conditionID: conditionID,
			assetID: assetID,
			negRisk: market.NegRisk,
			prices: deque.Deque[priceEvent]{},
			asks: treemap.NewWith(decimalComparator),
			bids: treemap.NewWith(decimalComparator),
		}
	}
	return subscription, true
}

func (s *marketSubscription) add(event priceEvent) {
	priceMin := decimal.Zero
	priceMax := decimalConstant("1.0")
	if event.price.LessThanOrEqual(priceMin) || event.price.GreaterThanOrEqual(priceMax) {
		log.Printf("Warning: invalid price for %s: %s", s.slug, event.price)
		return
	}
	s.prices.PushBack(event)
	duration := time.Duration(*configuration.Data.BufferTimeSpan) * time.Second
	now := time.Now()
	for s.prices.Len() > 0 {
		price := s.prices.Front()
		age := now.Sub(price.timestamp)
		if age > duration {
			s.prices.PopFront()
		} else {
			break
		}
	}
}

func (s *marketSubscription) getVolume() decimal.Decimal {
	volume := decimal.Zero
	for event := range s.prices.Iter() {
		volume = volume.Add(event.price.Mul(event.size))
	}
	return volume
}

func (s *marketSubscription) printOrderBook() {
	log.Printf("LOB for %s:", s.slug)
	fmt.Printf("  Asks:\n")
	printBookSide(s.asks, false)
	fmt.Printf("  Bids:\n")
	printBookSide(s.bids, true)
}

func (s *marketSubscription) validateOrderBook() bool {
	if s.bids.Size() > 0 && s.asks.Size() > 0 {
		itBids := s.bids.Iterator()
		itBids.End()
		_ = itBids.Prev()
		highestBid := itBids.Key().(decimal.Decimal)
		itAsks := s.asks.Iterator()
		itAsks.Begin()
		_ = itAsks.Next()
		lowestAsk := itAsks.Key().(decimal.Decimal)
		if lowestAsk.LessThanOrEqual(highestBid) {
			log.Printf("Warning: invalid LOB state for %s", s.slug)
			s.printOrderBook()
			return false
		} else {
			return true
		}
	} else {
		return false
	}
}

func getPriceSize(priceString string, sizeString string) (decimal.Decimal, decimal.Decimal, error) {
	price, err := decimal.NewFromString(priceString)
	if err != nil {
		fmt.Printf("Failed to convert price \"%s\" to decimal", priceString)
		return decimal.Zero, decimal.Zero, err
	}
	size, err := decimal.NewFromString(sizeString)
	if err != nil {
		fmt.Printf("Failed to convert size \"%s\" to decimal", sizeString)
		return decimal.Zero, decimal.Zero, err
	}
	return price, size, nil
}

func getEventMarkets() ([]Market, map[string]string, error) {
	markets := []Market{}
	eventSlugMap := map[string]string{}
	for _, tagSlug := range configuration.Data.TagSlugs {
		events, err := getEvents(&tagSlug)
		if err != nil {
			return nil, nil, err
		}
		for _, event := range events {
			if !event.Active || event.Closed {
				continue
			}
			for _, market := range event.Markets {
				if !market.Active || market.Closed {
					continue
				}
				volume := decimal.NewFromFloat(market.Volume24Hr)
				if volume.LessThan(configuration.Data.MinVolume.Decimal) {
					continue
				}
				exists := commons.ContainsFunc(markets, func (m Market) bool {
					return m.ConditionID == market.ConditionID
				})
				if exists {
					continue
				}
				eventSlugMap[market.Slug] = event.Slug
				markets = append(markets, market)
			}
		}
	}
	slices.SortFunc(markets, func (a, b Market) int {
			return cmp.Compare(b.Volume24Hr, a.Volume24Hr)
	})
	if len(markets) > marketChannelLimit {
		markets = markets[:marketChannelLimit]
	}
	return markets, eventSlugMap, nil
}

func printMarketStats(markets []Market) {
	if len(markets) < 2 {
		log.Printf("Warning: not enough markets to analyze volume")
		return
	}
	first := markets[0]
	last := markets[len(markets) - 1]
	log.Printf("Market 24h volume range: %.2f - %.2f", last.Volume24Hr, first.Volume24Hr)
}

func getAssetIDs(markets []Market) []string {
	assetIDs := []string{}
	for _, market := range markets {
		yesTokenID, err := getCLOBTokenID(market, true)
		if err != nil {
			continue
		}
		assetIDs = append(assetIDs, yesTokenID)
	}
	return assetIDs
}

var clobTokenIdPattern = regexp.MustCompile(`\d+`)

func getCLOBTokenID(market Market, yes bool) (string, error) {
	tokenIDs := []string{}
	matches := clobTokenIdPattern.FindAllStringSubmatch(market.CLOBTokenIDs, -1)
	for _, match := range matches {
		tokenId := match[0]
		tokenIDs = append(tokenIDs, tokenId)
	}
	if len(tokenIDs) != 2 {
		err := fmt.Errorf("unable to extract token ID: slug = %s, yes = %t, CLOBTokenIDs = %s", market.Slug, yes, market.CLOBTokenIDs)
		log.Printf("Warning: %v", err)
		return "", err
	}
	if yes {
		return tokenIDs[0], nil
	} else {
		return tokenIDs[1], nil
	}
}

func putPriceLevels(destination *treemap.Map, source []OrderSummary) {
	destination.Clear()
	for _, summary := range source {
		price, size, err := getPriceSize(summary.Price, summary.Size)
		if err != nil {
			continue
		}
		destination.Put(price, size)
	}
}

func printBookSide(book *treemap.Map, bids bool) {
	if book.Size() > 0 {
		it := book.Iterator()
		it.End()
		offset := 0
		for it.Prev() {
			bidsMatch := bids && offset < bookSidePrintLimit
			asksMatch := !bids && book.Size() - offset <= bookSidePrintLimit
			if bidsMatch || asksMatch {
				price := it.Key().(decimal.Decimal)
				size := it.Value().(decimal.Decimal)
				fmt.Printf("    [%s] %s\n", price, size)
			}
			offset++
		}
		it = book.Iterator()
	} else {
		fmt.Print("    -\n")
	}
}

func decimalComparator(a, b any) int {
	decimal1 := a.(decimal.Decimal)
	decimal2 := b.(decimal.Decimal)
	return decimal1.Cmp(decimal2)
}