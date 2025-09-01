package cyclobs

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
	"github.com/gammazero/deque"
	"github.com/polymarket/go-order-utils/pkg/model"
	"github.com/shopspring/decimal"
)

const (
	eventsLimit = 50
	reconnectDelay = 10
	bookEvent = "book"
	priceChangeEvent = "price_change"
	lastTradePriceEvent = "last_trade_price"
	debugPriceChange = false
	debugLastTradePrice = false
	debugOrderBook = false
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
	negRisk bool
	prices deque.Deque[priceEvent]
	bids *treemap.Map
	asks *treemap.Map
	triggered bool
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

func DataMode() {
	runMode(systemDataMode)
}

func TriggerMode() {
	runMode(systemTriggerMode)
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
	markets, eventSlugMap, err := getMarkets()
	if err != nil {
		return
	}
	printMarketStats(markets)
	s.markets = markets
	assetIDs := getAssetIDs(markets)
	s.database.insertMarkets(markets, assetIDs, eventSlugMap)
	s.subscribe(assetIDs)
}

func (s *tradingSystem) runTriggerMode() {
	positions, err := getPositions()
	if err != nil {
		return
	}
	assetIDs := []string{}
	for _, trigger := range configuration.Triggers {
		slug := *trigger.Slug
		position, exists := find(positions, func (p Position) bool {
			return p.Slug == slug
		})
		assetID := position.Asset
		if !exists {
			log.Fatalf("Unable to find a position matching trigger slug \"%s\"", slug)
		}
		assetIDs = append(assetIDs, assetID)
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
	err := subscribeToMarkets(assetIDs, func (messages []BookMessage) {
		for _, message := range messages {
			s.onBookMessage(message)
		}
	})
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
		s.processTrigger(price, subscription)
	}
}

func (s *tradingSystem) processTrigger(price decimal.Decimal, subscription *marketSubscription) {
	if subscription.triggered {
		return
	}
	trigger, exists := findPointer(s.triggers, func (t triggerData) bool {
		return t.slug == subscription.slug
	})
	if !exists {
		log.Printf("Warning: received a book message without a matching trigger: subscription.slug = %s", subscription.slug)
	}
	definition := trigger.trigger
	takeProfit := definition.TakeProfit
	sellPosition := func (limit decimal.Decimal) {
		err := postOrder(trigger.slug, trigger.assetID, model.SELL, trigger.size, limit, subscription.negRisk, 0)
		if err != nil {
			log.Printf("Failed to execute order: %v", err)
		}
		trigger.triggered = true
	}
	if takeProfit != nil && price.GreaterThanOrEqual(takeProfit.Decimal) {
		log.Printf("Take profit has been triggered for \"%s\" at %s", trigger.slug, price)
		sellPosition(definition.TakeProfitLimit.Decimal)
	} else if price.LessThanOrEqual(definition.StopLoss.Decimal) {
		log.Printf("Stop-loss has been triggered for \"%s\" at %s", trigger.slug, price)
		sellPosition(definition.StopLoss.Decimal)
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
			triggered: false,
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

func getMarkets() ([]Market, map[string]string, error) {
	markets := []Market{}
	eventSlugMap := map[string]string{}
	for _, tagSlug := range configuration.Data.TagSlugs {
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
				if volume.LessThan(configuration.Data.MinVolume.Decimal) {
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