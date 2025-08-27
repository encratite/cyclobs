package cyclobs

import (
	"cmp"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/gammazero/deque"
	"github.com/polymarket/go-order-utils/pkg/model"
)

const (
	eventsLimit = 50
	reconnectDelay = 60
	bookEvent = "book"
	priceChangeEvent = "price_change"
	lastTradePriceEvent = "last_trade_price"
	invalidTriggerID = -1
	invalidFloat64 = -1.0
	debugPriceChange = false
	debugOrderBook = false
	bookSidePrintLimit = 5
)

type tradingSystem struct {
	markets []Market
	subscriptions map[string]marketSubscription
	positions int
	mutex sync.Mutex
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
	price float64
}

type positionState struct {
	size float64
	added time.Time
}

func RunSystem() {
	loadConfiguration()
	system := tradingSystem{
		markets: []Market{},
		subscriptions: map[string]marketSubscription{},
		positions: 0,
	}
	system.run()
}

func (s *tradingSystem) run() {
	go s.runCleaner()
	sleep := func () {
		time.Sleep(time.Duration(reconnectDelay) * time.Second)
	}
	for {
		markets, err := getMarkets()
		if err != nil {
			sleep()
		}
		printMarketStats(markets)
		s.markets = markets
		assetIDs := getAssetIDs(markets)
		err = subscribeToMarkets(assetIDs, func (messages []BookMessage) {
			for _, message := range messages {
				s.onBookMessage(message)
			}
		})
		if err != nil {
			log.Printf("Subscription error: %v\n", err)
		}
		sleep()
	}
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
				log.Printf("Detected new position in cleaner: slug = %s, size = %.2f, added = %s\n", position.Slug, position.Size, now)
				continue
			}
			if state.size != position.Size {
				log.Printf("Size of position %s has changed from %.2f to %.2f, resetting expiration\n", position.Slug, state.size, position.Size)
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
			log.Printf("Position %s has expired, closing it\n", position.Slug)
			size := int(position.Size)
			limit := max(position.CurPrice - *cleanerConfig.Tolerance, 0.05)
			orderExpiration := *cleanerConfig.Interval
			postOrder(position.Asset, model.SELL, size, limit, position.NegativeRisk, orderExpiration)
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
		s.onBookEvent(message, subscription)
	case priceChangeEvent:
		s.onPriceChange(message, subscription)
	case lastTradePriceEvent:
		s.onLastTradePrice(message, subscription)
	}
	_ = subscription.validateOrderBook()
	s.subscriptions[key] = subscription
}

func (s *tradingSystem) onBookEvent(message BookMessage, subscription marketSubscription) {
	putPriceLevels(subscription.bids, message.Bids)
	putPriceLevels(subscription.asks, message.Asks)
	if debugOrderBook {
		subscription.printOrderBook()
	}
}

func (s *tradingSystem) onPriceChange(message BookMessage, subscription marketSubscription) {
	for i, change := range message.Changes {
		if debugPriceChange {
			log.Printf("%s[%d]: slug = %s, price = %s, size = %s, side = %s\n", priceChangeEvent, i, subscription.slug, change.Price, change.Size, change.Side)
		}
		price, size := getPriceSize(change.Price, change.Size)
		if price == invalidFloat64 {
			continue
		}
		var side, otherSide *treemap.Map
		switch change.Side {
		case sideBuy:
			side = subscription.bids
			otherSide = subscription.asks
		case sideSell:
			side = subscription.asks
			otherSide = subscription.bids
		default:
			continue
		}
		if size > 0.0 {
			side.Put(price, size)
			otherSide.Remove(price)
		} else if size == 0.0 {
			side.Remove(price)
			otherSide.Remove(price)
		} else {
			log.Printf("Warning: negative price change\n")
			side.Remove(price)
			otherSide.Remove(price)
		}
	}
	if debugOrderBook {
		subscription.printOrderBook()
	}
}

func (s *tradingSystem) onLastTradePrice(message BookMessage, subscription marketSubscription) {
	assetID := message.AssetID
	price, err := stringToFloat(message.Price)
	if err != nil {
		log.Printf("Failed to read price: \"%s\"\n", message.Price)
		return
	}
	event := priceEvent{
		timestamp: time.Now(),
		price: price,
	}
	log.Printf("%s: slug = %s, price = %s, size = %s, side = %s\n", lastTradePriceEvent, subscription.slug, message.Price, message.Size, message.Side)
	subscription.add(event)
	if message.Side == sideBuy {
		triggerID, trigger := subscription.getMatchingTrigger()
		if triggerID != invalidTriggerID {
			positions := s.getPositions()
			if positions >= *configuration.PositionLimit {
				log.Printf("Warning: found matching trigger but there are already %d active positions\n", s.positions)
				return
			}
			log.Printf("Trigger %d activated for %s at %.3f\n", triggerID, subscription.slug, price)
			_ = postOrder(assetID, model.BUY, *trigger.Size, *trigger.Limit, subscription.negRisk, *configuration.OrderExpiration)
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
			log.Printf("Warning: unable to find matching market for asset ID %s\n", assetID)
			return marketSubscription{}, false
		}
		subscription = marketSubscription{
			slug: market.Slug,
			negRisk: market.NegRisk,
			prices: deque.Deque[priceEvent]{},
			asks: treemap.NewWith(utils.Float64Comparator),
			bids: treemap.NewWith(utils.Float64Comparator),
			triggered: []int{},
		}
	}
	return subscription, true
}

func (s *marketSubscription) add(event priceEvent) {
	if event.price <= 0.0 || event.price >= 1.0 {
		log.Printf("Warning: invalid price for %s: %.3f", s.slug, event.price)
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
			log.Printf("Warning: skipping trigger %d for %s because it had already been triggered for this subscription\n", triggerID, s.slug)
			continue
		}
		first, exists := s.getFirstPrice(trigger)
		if !exists {
			log.Printf("Warning: no price data available for %s\n", s.slug)
			return invalidTriggerID, Trigger{}
		}
		last := s.prices.Back()
		if last.timestamp.Before(first.timestamp) {
			log.Printf("Warning: inconsistent timestamps in price data for %s\n", s.slug)
			return invalidTriggerID, Trigger{}
		}
		delta := last.price - first.price
		if delta < *trigger.Delta {
			log.Printf("Info: delta from %s too low for trigger %d: delta = %.3f, trigger.Delta = %.2f\n", s.slug, triggerID, delta, *trigger.Delta)
			continue
		}
		if last.price < *trigger.MinPrice || last.price > *trigger.MaxPrice {
			log.Printf("Info: price of %s outside of range for trigger %d: price = %.3f, trigger.MinPrice = %.2f, trigger.MaxPrice = %.2f\n", s.slug, triggerID, last.price, *trigger.MinPrice, *trigger.MaxPrice)
			continue
		}
		return triggerID, trigger
	}
	return invalidTriggerID, Trigger{}
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
	log.Printf("LOB for %s:\n", s.slug)
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
		highestBid := itBids.Key().(float64)
		itAsks := s.asks.Iterator()
		itAsks.Begin()
		_ = itAsks.Next()
		lowestAsk := itAsks.Key().(float64)
		if lowestAsk <= highestBid {
			log.Printf("Invalid LOB state for %s\n", s.slug)
			s.printOrderBook()
			return false
		} else {
			return true
		}
	} else {
		return false
	}
}

func getPriceSize(priceString string, sizeString string) (float64, float64) {
	price, err := stringToFloat(priceString)
	if err != nil {
		fmt.Printf("Failed to convert price \"%s\" to float", priceString)
		return invalidFloat64, invalidFloat64
	}
	size, err := stringToFloat(sizeString)
	if err != nil {
		fmt.Printf("Failed to convert size \"%s\" to float", sizeString)
		return invalidFloat64, invalidFloat64
	}
	return price, size
}

func getMarkets() ([]Market, error) {
	markets := []Market{}
	for _, tagSlug := range configuration.TagSlugs {
		events, err := getEvents(tagSlug)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			for _, market := range event.Markets {
				if market.Volume24Hr > *configuration.MinVolume {
					markets = append(markets, market)
				}
			}
		}
	}
	slices.SortFunc(markets, func (a, b Market) int {
			return cmp.Compare(b.Volume24Hr, a.Volume24Hr)
	})
	if len(markets) > marketChannelLimit {
		markets = markets[:marketChannelLimit]
	}
	return markets, nil
}

func printMarketStats(markets []Market) {
	if len(markets) < 2 {
		log.Printf("Warning: not enough markets to analyze volume")
		return
	}
	first := markets[0]
	last := markets[len(markets) - 1]
	log.Printf("Market 24h volume range: %.2f - %.2f\n", last.Volume24Hr, first.Volume24Hr)
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
		log.Printf("Warning: Unable to extract token ID for market %s\n", market.Slug)
		return "", false
	}
	yesTokenID := tokenIDs[0]
	return yesTokenID, true
}

func putPriceLevels(destination *treemap.Map, source []OrderSummary) {
	destination.Clear()
	for _, summary := range source {
		price, size := getPriceSize(summary.Price, summary.Size)
		if price == invalidFloat64 {
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
				price := it.Key().(float64)
				size := it.Value().(float64)
				fmt.Printf("    [%.3f] %.2f\n", price, size)
			}
			offset++
		}
		it = book.Iterator()
	} else {
		fmt.Print("    -\n")
	}
}