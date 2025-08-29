package cyclobs

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/gammazero/deque"
	"github.com/shopspring/decimal"
	"github.com/polymarket/go-order-utils/pkg/model"
)

const (
	eventsLimit = 50
	reconnectDelay = 10
	bookEvent = "book"
	priceChangeEvent = "price_change"
	lastTradePriceEvent = "last_trade_price"
	invalidTriggerID = -1
	debugPriceChange = false
	debugLastTradePrice = false
	debugOrderBook = false
	debugTrigger = true
	bookSidePrintLimit = 5
)

type tradingSystem struct {
	markets []Market
	subscriptions map[string]marketSubscription
	positions int
	mutex sync.Mutex
	database databaseClient
}

type marketSubscription struct {
	slug string
	negRisk bool
	prices deque.Deque[priceEvent]
	bids *treemap.Map
	asks *treemap.Map
	triggered []int
}

type priceEvent struct {
	timestamp time.Time
	price decimal.Decimal
}

type positionState struct {
	size float64
	added time.Time
}

func RunSystem() {
	loadConfiguration()
	database := newDatabaseClient()
	system := tradingSystem{
		markets: []Market{},
		subscriptions: map[string]marketSubscription{},
		positions: 0,
		database: database,
	}
	system.run()
}

func (s *tradingSystem) run() {
	go s.runCleaner()
	sleep := func () {
		time.Sleep(time.Duration(reconnectDelay) * time.Second)
	}
	defer s.database.close()
	s.interrupt()
	for {
		markets, eventSlugMap, err := getMarkets()
		if err != nil {
			sleep()
		}
		printMarketStats(markets)
		s.markets = markets
		assetIDs := getAssetIDs(markets)
		s.database.insertMarkets(markets, assetIDs, eventSlugMap)
		err = subscribeToMarkets(assetIDs, func (messages []BookMessage) {
			for _, message := range messages {
				s.onBookMessage(message)
			}
		})
		if err != nil {
			log.Printf("Subscription error: %v", err)
		}
		sleep()
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

func (s *tradingSystem) runCleaner() {
	cleanerConfig := configuration.Cleaner
	states := map[string]positionState{}
	sleep := func() {
		duration := time.Duration(*cleanerConfig.Interval) * time.Second
		time.Sleep(duration)
	}
	for {
		positions, err := getPositions()
		if err != nil {
			sleep()
			continue
		}
		s.setPositions(len(positions))
		now := time.Now()
		positionExpiration := time.Duration(*cleanerConfig.Expiration) * time.Second
		for _, position := range positions {
			state, exists := states[position.Asset]
			if !exists {
				state = positionState{
					size: position.Size,
					added: now,
				}
				states[position.Asset] = state
				log.Printf("Detected new position in cleaner: slug = %s, size = %.2f, added = %s", position.Slug, position.Size, now)
				continue
			}
			if state.size != position.Size {
				log.Printf("Size of position %s has changed from %.2f to %.2f, resetting expiration", position.Slug, state.size, position.Size)
				states[position.Asset] = positionState{
					size: position.Size,
					added: now,
				}
				continue
			}
			age := now.Sub(state.added)
			if age < positionExpiration {
				continue
			}
			log.Printf("Position %s has expired, closing it", position.Slug)
			size := int(position.Size)
			decimalPrice := decimal.NewFromFloat(position.CurPrice)
			limit := decimalPrice.Sub(cleanerConfig.LimitOffset.Decimal)
			limitMin := decimalConstant("0.05")
			if limit.LessThan(limitMin) {
				limit = limitMin
			}
			orderExpiration := *cleanerConfig.Interval
			postOrder(position.Slug, position.Asset, model.SELL, size, limit, position.NegativeRisk, orderExpiration)
		}
		sleep()
	}
}

func (s *tradingSystem) getPositions() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.positions
}

func (s *tradingSystem) setPositions(positions int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.positions = positions
}

func (s *tradingSystem) onBookMessage(message BookMessage) {
	key := message.AssetID
	subscription, exists := s.getSubscription(key)
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
	s.database.insertBookMessage(message, subscription)
	_ = subscription.validateOrderBook()
	s.subscriptions[key] = subscription
}

func (s *tradingSystem) onBookEvent(message BookMessage, subscription *marketSubscription) {
	putPriceLevels(subscription.bids, message.Bids)
	putPriceLevels(subscription.asks, message.Asks)
	if debugOrderBook {
		subscription.printOrderBook()
	}
}

func (s *tradingSystem) onPriceChange(message BookMessage, subscription *marketSubscription) {
	for i, change := range message.Changes {
		if debugPriceChange {
			log.Printf("%s[%d]: slug = %s, price = %s, size = %s, side = %s", priceChangeEvent, i, subscription.slug, change.Price, change.Size, change.Side)
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
	assetID := message.AssetID
	price, err := decimal.NewFromString(message.Price)
	if err != nil {
		log.Printf("Failed to read price: \"%s\"", message.Price)
		return
	}
	event := priceEvent{
		timestamp: time.Now(),
		price: price,
	}
	if debugLastTradePrice {
		log.Printf("%s: slug = %s, price = %s, size = %s, side = %s", lastTradePriceEvent, subscription.slug, message.Price, message.Size, message.Side)
	}
	subscription.add(event)
	if message.Side == sideBuy {
		triggerID, trigger := subscription.getMatchingTrigger()
		if triggerID != invalidTriggerID {
			positions := s.getPositions()
			if positions >= *configuration.PositionLimit {
				log.Printf("Warning: found matching trigger but there are already %d active positions", s.positions)
				return
			}
			log.Printf("Trigger %d activated for %s at %s", triggerID, subscription.slug, price)
			orderPrice := price.Add(trigger.LimitOffset.Decimal)
			_ = postOrder(subscription.slug, assetID, model.BUY, *trigger.Size, orderPrice, subscription.negRisk, *configuration.OrderExpiration)
			subscription.setTriggered(triggerID)
		}
	}
}

func (s *tradingSystem) getMarket(assetID string) (Market, bool) {
	market, exists := find(s.markets, func (market Market) bool {
		yesTokenID, exists := getYesTokenID(market)
		if !exists {
			return false
		}
		return yesTokenID == assetID
	})
	if exists {
		return market, true
	} else {
		return Market{}, false
	}
}

func (s *tradingSystem) getSubscription(assetID string) (marketSubscription, bool) {
	subscription, success := s.subscriptions[assetID]
	if !success {
		market, exists := s.getMarket(assetID)
		if !exists {
			log.Printf("Warning: unable to find matching market for asset ID %s", assetID)
			return marketSubscription{}, false
		}
		subscription = marketSubscription{
			slug: market.Slug,
			negRisk: market.NegRisk,
			prices: deque.Deque[priceEvent]{},
			asks: treemap.NewWith(decimalComparator),
			bids: treemap.NewWith(decimalComparator),
			triggered: []int{},
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
	timeSpans := []int{}
	for _, trigger := range configuration.Triggers {
		timeSpans = append(timeSpans, *trigger.TimeSpan)
	}
	maxTimeSpan := slices.Max(timeSpans)
	duration := time.Duration(maxTimeSpan) * time.Second
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

func (s *marketSubscription) getMatchingTrigger() (int, Trigger) {
	if s.prices.Len() < 2 {
		return invalidTriggerID, Trigger{}	
	}
	for triggerID, trigger := range configuration.Triggers {
		triggered := contains(s.triggered, triggerID)
		if triggered {
			log.Printf("Warning: skipping trigger %d for %s because it had already been triggered for this subscription", triggerID, s.slug)
			continue
		}
		first, exists := s.getFirstPrice(trigger)
		if !exists {
			log.Printf("Warning: no price data available for %s", s.slug)
			return invalidTriggerID, Trigger{}
		}
		last := s.prices.Back()
		if last.timestamp.Before(first.timestamp) {
			log.Printf("Warning: inconsistent timestamps in price data for %s", s.slug)
			return invalidTriggerID, Trigger{}
		}
		delta := last.price.Sub(first.price)
		if delta.LessThan(trigger.Delta.Decimal) {
			if debugTrigger {
				format := "Info: delta from %s too low for trigger %d: first.timestamp = %s, first.price = %s, last.timestamp = %s, last.price = %s, delta = %s, trigger.Delta = %s"
				log.Printf(format, s.slug, triggerID, getTimeString(first.timestamp), first.price, getTimeString(last.timestamp), last.price, delta, *trigger.Delta)
			}
			continue
		}
		if last.price.LessThan(trigger.MinPrice.Decimal) || last.price.GreaterThan(trigger.MaxPrice.Decimal) {
			if debugTrigger {
				format := "Info: price of %s outside of range for trigger %d: price = %s, trigger.MinPrice = %s, trigger.MaxPrice = %s"
				log.Printf(format, s.slug, triggerID, last.price, *trigger.MinPrice, *trigger.MaxPrice)
			}
			continue
		}
		bidLiquidity, askLiquidity := s.getLiquidity(last.price, trigger.LiquidityRange.Decimal)
		if bidLiquidity.LessThan(trigger.MinLiquidity.Decimal) || askLiquidity.LessThan(trigger.MinLiquidity.Decimal) {
			if debugTrigger {
				format := "Info: liquidity requirements for %s were not met: bidLiquidity = %s, askLiquidity = %s, trigger.MinLiquidity = %s"
				log.Printf(format, s.slug, bidLiquidity, askLiquidity, *trigger.MinLiquidity)
			}
			continue
		}
		valid := s.validateOrderBook()
		if !valid {
			return invalidTriggerID, Trigger{}
		}
		return triggerID, trigger
	}
	return invalidTriggerID, Trigger{}
}

func (s *marketSubscription) getLiquidity(lastTradePrice, liquidityRange decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	bidLiquidity := decimal.Zero
	itBids := s.bids.Iterator()
	itBids.End()
	for itBids.Prev() {
		priceLevel := itBids.Key().(decimal.Decimal)
		size := itBids.Key().(decimal.Decimal)
		priceLevelDelta := lastTradePrice.Sub(priceLevel)
		if priceLevelDelta.GreaterThan(liquidityRange) {
			break
		}
		bidLiquidity = bidLiquidity.Add(priceLevel.Mul(size))
	}
	askLiquidity := decimal.Zero
	itAsks := s.asks.Iterator()
	for itAsks.Next() {
		priceLevel := itBids.Key().(decimal.Decimal)
		size := itBids.Key().(decimal.Decimal)
		priceLevelDelta := priceLevel.Sub(lastTradePrice)
		if priceLevelDelta.GreaterThan(liquidityRange) {
			break
		}
		askLiquidity = askLiquidity.Add(priceLevel.Mul(size))
	}
	return bidLiquidity, askLiquidity
}

func (s *marketSubscription) setTriggered(triggerID int) {
	if contains(s.triggered, triggerID) {
		return
	}
	s.triggered = append(s.triggered, triggerID)
}

func (s *marketSubscription) getFirstPrice(trigger Trigger) (priceEvent, bool) {
	now := time.Now()
	timeSpan := time.Duration(*trigger.TimeSpan) * time.Second
	for price := range s.prices.Iter() {
		age := now.Sub(price.timestamp)
		if age < timeSpan {
			return price, true
		}
	}
	return priceEvent{}, false
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

func getMarkets() ([]Market, map[string]string, error) {
	markets := []Market{}
	eventSlugMap := map[string]string{}
	for _, tagSlug := range configuration.TagSlugs {
		events, err := getEvents(tagSlug)
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
				if volume.LessThan(configuration.MinVolume.Decimal) {
					continue
				}
				exists := containsFunc(markets, func (m Market) bool {
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
		yesTokenID, exists := getYesTokenID(market)
		if !exists {
			continue
		}
		assetIDs = append(assetIDs, yesTokenID)
	}
	return assetIDs
}

var clobTokenIdPattern = regexp.MustCompile(`\d+`)

func getCLOBTokenIds(market Market) []string {
	tokenIds := []string{}
	matches := clobTokenIdPattern.FindAllStringSubmatch(market.CLOBTokenIDs, -1)
	for _, match := range matches {
		tokenId := match[0]
		tokenIds = append(tokenIds, tokenId)
	}
	return tokenIds
}

func getYesTokenID(market Market) (string, bool) {
	tokenIDs := getCLOBTokenIds(market)
	if len(tokenIDs) != 2 {
		log.Printf("Warning: Unable to extract token ID for market %s", market.Slug)
		return "", false
	}
	yesTokenID := tokenIDs[0]
	return yesTokenID, true
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