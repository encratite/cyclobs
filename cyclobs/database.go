package cyclobs

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	databaseTimeout = 5
	millisecondsPerSecond = 1000
	databaseBookDepth = 10
	priceChangeBufferLimit = 250
	marketCollection = "markets"
	marketVolumeCollection = "market_volume"
	bookEventCollection = "book_events"
	priceChangeCollection = "price_changes"
	lastTradePriceCollection = "last_trade_prices"
	historyCollection = "history"
)

type databaseClient struct {
	client *mongo.Client
	database *mongo.Database
	markets *mongo.Collection
	marketVolume *mongo.Collection
	bookEvents *mongo.Collection
	priceChanges *mongo.Collection
	lastTradePrices *mongo.Collection
	history *mongo.Collection
	priceChangeBuffer []PriceChangeBSON
}

type MarketBSON struct {
	Slug string `bson:"slug"`
	Event string `bson:"event"`
	AssetID string `bson:"asset_id"`
	NegRisk bool `bson:"neg_risk"`
	Added time.Time `bson:"added"`
}

type MarketVolume struct {
	Slug string `bson:"slug"`
	Timestamp time.Time `bson:"timestamp"`
	Volume float64 `bson:"volume"`
}

type BookEvent struct {
	AssetID string `bson:"asset_id"`
	ServerTime time.Time `bson:"server_time"`
	LocalTime time.Time `bson:"local_time"`
	Bids []PriceLevel `bson:"bids"`
	Asks []PriceLevel `bson:"asks"`
}

type PriceChangeBSON struct {
	AssetID string `bson:"asset_id"`
	ServerTime time.Time `bson:"server_time"`
	LocalTime time.Time `bson:"local_time"`
	Price bson.Decimal128 `bson:"price"`
	Size bson.Decimal128 `bson:"size"`
	Buy bool `bson:"buy"`
}

type LastTradePrice struct {
	AssetID string `bson:"asset_id"`
	ServerTime time.Time `bson:"server_time"`
	LocalTime time.Time `bson:"local_time"`
	Price bson.Decimal128 `bson:"price"`
	Size bson.Decimal128 `bson:"size"`
	Buy bool `bson:"buy"`
	Volume bson.Decimal128 `bson:"volume"`
	Bids []PriceLevel `bson:"bids"`
	Asks []PriceLevel `bson:"asks"`
}

type PriceLevel struct {
	Price bson.Decimal128 `bson:"price"`
	Size bson.Decimal128 `bson:"size"`
}

type PriceHistoryBSON struct {
	Slug string `bson:"slug"`
	Closed bool `bson:"closed"`
	Outcome *bool `bson:"outcome"`
	Tags []string `bson:"tags"`
	History []PriceHistorySampleBSON `bson:"history"`
}

type PriceHistorySampleBSON struct {
	Timestamp time.Time `bson:"timestamp"`
	Price float64 `bson:"price"`
}

func newDatabaseClient() databaseClient {
	clientOptions := options.Client().ApplyURI(*configuration.Database.URI)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	database := client.Database(*configuration.Database.Database)
	markets := database.Collection(marketCollection)
	marketVolume := database.Collection(marketVolumeCollection)
	bookEvents := database.Collection(bookEventCollection)
	priceChanges := database.Collection(priceChangeCollection)
	lastTradePrices := database.Collection(lastTradePriceCollection)
	history := database.Collection(historyCollection)
	dbClient := databaseClient{
		client: client,
		database: database,
		markets: markets,
		marketVolume: marketVolume,
		bookEvents: bookEvents,
		priceChanges: priceChanges,
		lastTradePrices: lastTradePrices,
		history: history,
		priceChangeBuffer: []PriceChangeBSON{},
	}
	dbClient.createIndexes()
	return dbClient
}

func (c *databaseClient) createIndexes() {	
	c.createMarketIndexes()
	c.createChannelIndexes()
	c.createOtherIndexes()
}

func (c *databaseClient) createMarketIndexes() {
	slugKey := bson.D{
		{Key: "slug", Value: 1},
	}
	marketSlugIndex := mongo.IndexModel{
		Keys: slugKey,
		Options: options.Index().SetUnique(true),
	}
	createIndex(c.markets, marketSlugIndex)
	assetKey := bson.D{
		{Key: "asset_id", Value: 1},
	}
	marketAssetIndex := mongo.IndexModel{
		Keys: assetKey,
		Options: options.Index().SetUnique(true),
	}
	createIndex(c.markets, marketAssetIndex)
	marketVolumeIndex := mongo.IndexModel{
		Keys: slugKey,
	}
	createIndex(c.marketVolume, marketVolumeIndex)
}

func (c *databaseClient) createChannelIndexes() {
	keys := bson.D{
		{Key: "asset_id", Value: 1},
		{Key: "server_time", Value: 1},
	}
	indexModel := mongo.IndexModel{
		Keys: keys,
	}
	collections := []*mongo.Collection{
		c.bookEvents,
		c.priceChanges,
		c.lastTradePrices,
	}
	for _, collection := range collections {
		createIndex(collection, indexModel)
	}
}

func (c *databaseClient) createOtherIndexes() {
	slugKey := bson.D{
		{Key: "slug", Value: 1},
	}
	historySlugIndex := mongo.IndexModel{
		Keys: slugKey,
		Options: options.Index().SetUnique(true),
	}
	createIndex(c.history, historySlugIndex)
}

func (c *databaseClient) close() {
	ctx, cancel := getDatabaseContext()
	defer cancel()
	c.client.Disconnect(ctx)
}

func (c *databaseClient) insertMarkets(markets []Market, assetIDs []string, eventSlugMap map[string]string) {
	dbMarkets := []MarketBSON{}
	dbVolume := []MarketVolume{}
	now := time.Now()
	for i, market := range markets {
		event, exists := eventSlugMap[market.Slug]
		if !exists {
			log.Printf("Warning: unable to determine event slug for market %s", market.Slug)
			continue
		}
		assetID := assetIDs[i]
		dbMarket := MarketBSON{
			Slug: market.Slug,
			Event: event,
			AssetID: assetID,
			NegRisk: market.NegRisk,
			Added: now,
		}
		dbMarkets = append(dbMarkets, dbMarket)
		volume := MarketVolume{
			Slug: market.Slug,
			Timestamp: now,
			Volume: market.Volume24Hr,
		}
		dbVolume = append(dbVolume, volume)
	}
	ctx, cancel := getDatabaseContext()
	defer cancel()
	ordered := options.InsertMany().SetOrdered(false)
	c.markets.InsertMany(ctx, dbMarkets, ordered)
	_, err := c.marketVolume.InsertMany(ctx, dbVolume)
	if err != nil {
		log.Printf("Failed to insert volume data: %v", err)
	}
}

func (c *databaseClient) insertBookMessage(message BookMessage, subscription marketSubscription) {
	switch message.EventType {
	case bookEvent:
		c.insertBookEvent(message)
	case priceChangeEvent:
		c.insertPriceChange(message)
	case lastTradePriceEvent:
		c.insertLastTradePrice(message, subscription)
	}
}

func (c *databaseClient) insertBookEvent(message BookMessage) {
	serverTime, err := convertTimestampString(message.Timestamp)
	if err != nil {
		return
	}
	localTime := time.Now()
	bids, err := convertOrderSummaries(message.Bids)
	if err != nil {
		return
	}
	asks, err := convertOrderSummaries(message.Asks)
	if err != nil {
		return
	}
	bookEvent := BookEvent{
		AssetID: message.AssetID,
		ServerTime: serverTime,
		LocalTime: localTime,
		Bids: bids,
		Asks: asks,
	}
	ctx, cancel := getDatabaseContext()
	defer cancel()
	_, insertErr := c.bookEvents.InsertOne(ctx, bookEvent)
	if insertErr != nil {
		log.Printf("Warning: failed to insert book event into database: %v", err)
	}
}

func (c *databaseClient) insertPriceChange(message BookMessage) {
	serverTime, err := convertTimestampString(message.Timestamp)
	if err != nil {
		return
	}
	localTime := time.Now()
	for _, change := range message.Changes {
		price, size, err := convertPriceSize(change.Price, change.Size)
		if err != nil {
			return
		}
		buy, err := convertSide(change.Side)
		if err != nil {
			return
		}
		priceChange := PriceChangeBSON{
			AssetID: message.AssetID,
			ServerTime: serverTime,
			LocalTime: localTime,
			Price: price,
			Size: size,
			Buy: buy,
		}
		c.priceChangeBuffer = append(c.priceChangeBuffer, priceChange)
	}
	if len(c.priceChangeBuffer) >= priceChangeBufferLimit {
		c.flushBuffer()
	}
}

func (c *databaseClient) insertLastTradePrice(message BookMessage, subscription marketSubscription) {
	serverTime, err := convertTimestampString(message.Timestamp)
	if err != nil {
		return
	}
	localTime := time.Now()
	price, size, err := convertPriceSize(message.Price, message.Size)
	if err != nil {
		return
	}
	buy, err := convertSide(message.Side)
	if err != nil {
		return
	}
	volume, err := convertVolume(subscription)
	if err != nil {
		return
	}
	bids := getPriceLevels(subscription.bids, true)
	asks := getPriceLevels(subscription.asks, false)
	lastTradePrice := LastTradePrice{
		AssetID: message.AssetID,
		ServerTime: serverTime,
		LocalTime: localTime,
		Price: price,
		Size: size,
		Buy: buy,
		Volume: volume,
		Bids: bids,
		Asks: asks,
	}
	ctx, cancel := getDatabaseContext()
	defer cancel()
	_, insertErr := c.lastTradePrices.InsertOne(ctx, lastTradePrice)
	if insertErr != nil {
		log.Printf("Warning: failed to insert last trade price into database: %v", err)
	}
}

func (c *databaseClient) priceHistoryExists(slug string) bool {
	filter := bson.M{
		"slug": slug,
	}
	projection := bson.M{
		"_id": 1,
	}
	opts := options.FindOne().SetProjection(projection)
	ctx, cancel := getDatabaseContext()
	defer cancel()
	err := c.history.FindOne(ctx, filter, opts).Err()
	if err == mongo.ErrNoDocuments {
		return false
	} else if err != nil {
		log.Printf("Failed to determine if price history exists: %v", err)
		return true
	} else {
		return true
	}
}

func (c *databaseClient) insertPriceHistory(history PriceHistoryBSON) {
	ctx, cancel := getDatabaseContext()
	defer cancel()
	_, err := c.history.InsertOne(ctx, history)
	if err != nil {
		log.Printf("Warning: failed to insert price history into database: %v", err)
	}
}

func (c *databaseClient) getPriceHistoryData() []PriceHistoryBSON {
	ctx, cancel := getDatabaseContext()
	defer cancel()
	cursor, err := c.history.Find(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to read price history data: %v", err)
	}
	defer cursor.Close(ctx)
	var historyData []PriceHistoryBSON
	if err := cursor.All(ctx, &historyData); err != nil {
		log.Fatalf("Failed to iterate over cursor: %v", err)
	}
	return historyData
}

func (c *databaseClient) flushBuffer() {
	ctx, cancel := getDatabaseContext()
	defer cancel()
	_, insertErr := c.priceChanges.InsertMany(ctx, c.priceChangeBuffer)
	if insertErr != nil {
		log.Printf("Warning: failed to insert price change into database: %v", insertErr)
	}
	c.priceChangeBuffer = c.priceChangeBuffer[:0]
}

func convertTimestampString(timestamp string) (time.Time, error) {
	milliseconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		log.Printf("Warning: failed to convert timestamp in book message: %s", timestamp)
		return time.Time{}, err
	}
	seconds := milliseconds / millisecondsPerSecond
	nanoseconds := (milliseconds % millisecondsPerSecond) * int64(time.Millisecond)
	output := time.Unix(seconds, nanoseconds).UTC()
	return output, nil
}

func convertSide(side string) (bool, error) {
	switch side {
	case sideBuy:
		return true, nil
	case sideSell:
		return false, nil
	default:
		err := fmt.Errorf("failed to convert side string in book message: %s", side)
		log.Printf("Warning: %v", err)
		return false, err
	}
}

func convertOrderSummaries(summaries []OrderSummary) ([]PriceLevel, error) {
	priceLevels := []PriceLevel{}
	for _, summary := range summaries {
		price, size, err := convertPriceSize(summary.Price, summary.Size)
		if err != nil {
			return nil, err
		}
		priceLevel := PriceLevel{
			Price: price,
			Size: size,
		}
		priceLevels = append(priceLevels, priceLevel)
	}
	return priceLevels, nil
}

func convertPriceSize(price, size string) (bson.Decimal128, bson.Decimal128, error) {
	priceDecimal, err := bson.ParseDecimal128(price)
	if err != nil {
		log.Printf("Warning: failed to convert price in book message: %s", price)
		return bson.Decimal128{}, bson.Decimal128{}, err
	}
	sizeDecimal, err := bson.ParseDecimal128(size)
	if err != nil {
		log.Printf("Warning: failed to convert size in book message: %s", size)
		return bson.Decimal128{}, bson.Decimal128{}, err
	}
	return priceDecimal, sizeDecimal, nil
}

func convertVolume(subscription marketSubscription) (bson.Decimal128, error) {
	volume := subscription.getVolume()
	volumeDecimal, err := bson.ParseDecimal128(volume.String())
	if err != nil {
		log.Printf("Warning: failed to convert volume: %s", volume)
		return bson.Decimal128{}, err
	}
	return volumeDecimal, nil
}

func getPriceLevels(book *treemap.Map, bids bool) []PriceLevel {
	it := book.Iterator()
	it.End()
	offset := 0
	priceLevels := []PriceLevel{}
	for it.Prev() {
		bidsMatch := bids && offset < databaseBookDepth
		asksMatch := !bids && book.Size() - offset <= databaseBookDepth
		if bidsMatch || asksMatch {
			price := it.Key().(decimal.Decimal)
			size := it.Value().(decimal.Decimal)
			priceDecimal, _ := bson.ParseDecimal128(price.String())
			sizeDecimal, _ := bson.ParseDecimal128(size.String())
			priceLevel := PriceLevel{
				Price: priceDecimal,
				Size: sizeDecimal,
			}
			priceLevels = append(priceLevels, priceLevel)
		}
		offset++
	}
	it = book.Iterator()
	return priceLevels
}

func getDatabaseContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout * time.Second)
	return ctx, cancel
}

func createIndex(collection *mongo.Collection, indexModel mongo.IndexModel) {
	ctx, cancel := getDatabaseContext()
	defer cancel()
	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
}