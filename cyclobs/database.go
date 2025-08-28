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
	bookEventCollection = "book"
	priceChangeCollection = "price_change"
	lastTradePriceCollection = "last_trade_price"
)

type databaseClient struct {
	ctx context.Context
	cancel context.CancelFunc
	client *mongo.Client
	database *mongo.Database
	bookEvent *mongo.Collection
	priceChange *mongo.Collection
	lastTradePrice *mongo.Collection
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
	Bids []PriceLevel `bson:"bids"`
	Asks []PriceLevel `bson:"asks"`
}

type PriceLevel struct {
	Price bson.Decimal128 `bson:"price"`
	Size bson.Decimal128 `bson:"size"`
}

func newDatabaseClient() databaseClient {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout * time.Second)
	options := options.Client().ApplyURI(*configuration.Database.URI)
	client, err := mongo.Connect(options)
	if err != nil {
		log.Fatal(err)
	}
	database := client.Database(*configuration.Database.Database)
	bookEvent := database.Collection(bookEventCollection)
	priceChange := database.Collection(priceChangeCollection)
	lastTradePrice := database.Collection(lastTradePriceCollection)
	return databaseClient{
		ctx: ctx,
		cancel: cancel,
		client: client,
		database: database,
		bookEvent: bookEvent,
		priceChange: priceChange,
		lastTradePrice: lastTradePrice,
	}
}

func (c *databaseClient) close() {
	c.cancel()
	c.client.Disconnect(c.ctx)
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
	_, insertErr := c.bookEvent.InsertOne(c.ctx, bookEvent)
	if insertErr != nil {
		log.Printf("Warning: failed to insert book event into database: %v\n", err)
	}
}

func (c *databaseClient) insertPriceChange(message BookMessage) {
	priceChange, err := getPriceChange(message)
	if err != nil {
		return
	}
	_, insertErr := c.priceChange.InsertOne(c.ctx, priceChange)
	if insertErr != nil {
		log.Printf("Warning: failed to price change into database: %v\n", err)
	}
}

func (c *databaseClient) insertLastTradePrice(message BookMessage, subscription marketSubscription) {
	priceChange, err := getPriceChange(message)
	if err != nil {
		return
	}
	bids := getPriceLevels(subscription.bids, true)
	asks := getPriceLevels(subscription.asks, false)
	lastTradePrice := LastTradePrice{
		AssetID: message.AssetID,
		ServerTime: priceChange.ServerTime,
		LocalTime: priceChange.LocalTime,
		Price: priceChange.Price,
		Size: priceChange.Size,
		Buy: priceChange.Buy,
		Bids: bids,
		Asks: asks,
	}
	_, insertErr := c.lastTradePrice.InsertOne(c.ctx, lastTradePrice)
	if insertErr != nil {
		log.Printf("Warning: failed to last trade price into database: %v\n", err)
	}
}

func convertTimestampString(timestamp string) (time.Time, error) {
	milliseconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		log.Printf("Warning: failed to convert timestamp in book message: %s\n", timestamp)
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
		log.Printf("Warning: %v\n", err)
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

func getPriceChange(message BookMessage) (PriceChangeBSON, error) {
	serverTime, err := convertTimestampString(message.Timestamp)
	if err != nil {
		return PriceChangeBSON{}, err
	}
	localTime := time.Now()
	price, size, err := convertPriceSize(message.Price, message.Size)
	if err != nil {
		return PriceChangeBSON{}, err
	}
	buy, err := convertSide(message.Side)
	if err != nil {
		return PriceChangeBSON{}, err
	}
	priceChange := PriceChangeBSON{
		AssetID: message.AssetID,
		ServerTime: serverTime,
		LocalTime: localTime,
		Price: price,
		Size: size,
		Buy: buy,
	}
	return priceChange, nil
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