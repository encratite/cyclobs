package cyclobs

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
)

const eventsLimit = 50
const marketChannelLimit = 500
const priceChangeEvent = "price_change"
const lastTradePriceEvent = "last_trade_price"
const chainId = 137
const baseTicks = 10000
const winnerTicks = 100 * baseTicks
const centsPerDollar = 100.0
const hexPrefix = "0x"

type NewOrder struct {
	DeferExec bool `json:"deferExec"`
	Order Order `json:"order"`
	Owner string `json:"owner"`
	OrderType string `json:"orderType"`
}

type Order struct {
	Salt int64 `json:"salt"`
	Maker string `json:"maker"`
	Signer string `json:"signer"`
	Taker string `json:"taker"`
	TokenID string `json:"tokenId"`
	MakerAmount string `json:"makerAmount"`
	TakerAmount string `json:"takerAmount"`
	Side string `json:"side"`
	Expiration string `json:"expiration"`
	Nonce string `json:"nonce"`
	FeeRateBPs string `json:"feeRateBps"`
	SignatureType int `json:"signatureType"`
	Signature string `json:"signature"`
}

func RunService() {
	loadConfiguration()
	events := getEvents("economy")
	if events != nil {
		fmt.Printf("Received %d events\n", len(events))
	}
	assetIDs := []string{}
	markets := []Market{}
	minTickSizes := map[float64]int{}
	for _, event := range events {
		for _, market := range event.Markets {
			tokenIDs := getCLOBTokenIds(market)
			if len(tokenIDs) != 2 {
				// log.Printf("Invalid CLOB token ID string for market \"%s\": \"%s\"", market.Slug, market.CLOBTokenIDs)
				continue
			}
			minTickSizes[market.OrderPriceMinTickSize] += 1
			yesTokenID := tokenIDs[0]
			if market.Slug == "fed-decreases-interest-rates-by-25-bps-after-september-2025-meeting" {
				fmt.Printf("Token ID: %s\n", yesTokenID)
			}
			assetIDs = append(assetIDs, yesTokenID)
			markets = append(markets, market)
		}
	}
	// subscribeToMarkets(assetIDs, markets)
	tokenID := "56831000532202254811410354120402056896323359630546371545035370679912675847818"
	postOrder(tokenID, 5, 0.07)
}

func getEvents(tagSlug string) []Event {
	base := "https://gamma-api.polymarket.com/events/pagination"
	u, err := url.Parse(base)
	if err != nil {
		log.Fatalf("Unable to parse URL (%s): %v", base, err)
	}
	values := url.Values{}
	values.Add("limit", strconv.FormatInt(eventsLimit, 10))
	values.Add("archived", "false")
	values.Add("tag_slug", tagSlug)
	values.Add("order", "volume24hr")
	values.Add("ascending", "false")
	u.RawQuery = values.Encode()
	encoded := u.String()
	response, err := http.Get(encoded)
	if err != nil {
		log.Printf("Failed to GET markets (%s): %v", encoded, err)
		return nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response (%s): %v", encoded, err)
		return nil
	}
	var eventsResponse EventsResponse
	err = json.Unmarshal(body, &eventsResponse)
	if err != nil {
		log.Printf("Failed to parse market JSON data (%s): %v", encoded, err)
		return nil
	}
	return eventsResponse.Data
}

func subscribeToMarkets(assetIDs []string, markets []Market) {
	if len(assetIDs) > marketChannelLimit || len(markets) > marketChannelLimit {
		log.Fatalf("Too many markets to subscribe to (%d)", len(assetIDs))
	}
	url := "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Printf("Failed to connect to market channel: %v", err)
		return
	}
	defer connection.Close()
	subscription := Subscription{
		AssetIDs: &assetIDs,
		Type: "market",
	}
	subscriptionData, err := json.Marshal(subscription)
	if err != nil {
		log.Printf("Failed to serialize subscription object: %v\n", err)
		return
	}
	err = connection.WriteMessage(websocket.TextMessage, subscriptionData)
	if err != nil {
		log.Printf("Failed to send subscription data: %v\n", err)
		return
	}
	go func () {
		for {
			pingData := []byte("PING")
			err := connection.WriteMessage(websocket.TextMessage, pingData)
			if err != nil {
				log.Printf("Failed to send ping: %v\n", err)
				break
			}
			time.Sleep(10 * time.Second)
		}
	}()
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			log.Printf("Failed to read message: %v\n", err)
			return
		}
		messageString := string(message)
		if messageString == "PONG" {
			continue
		}
		var bookMessages []BookMessage
		err = json.Unmarshal(message, &bookMessages)
		if err != nil {
			log.Printf("Failed to deserialize book message: %v\n", err)
			log.Printf("Message: %s\n", messageString)
			return
		}
		if len(bookMessages) > 1 {
			fmt.Printf("Received %d book messages\n", len(bookMessages))
		}
		for _, bookMessage := range bookMessages {
			market, exists := find(markets, func (m Market) bool {
				return m.ConditionID == bookMessage.Market
			})
			if !exists {
				continue
			}
			if bookMessage.EventType == priceChangeEvent && len(bookMessage.Changes) > 0 {
				// change := bookMessage.Changes[0]
				// log.Printf("Price change for market \"%s\": size = %s, price = %s, side = %s", market.Slug, change.Size, change.Price, change.Side)
			} else if bookMessage.EventType == lastTradePriceEvent {
				log.Printf("Last trade price for market \"%s\": size = %s, price = %s, side = %s", market.Slug, bookMessage.Size, bookMessage.Price, bookMessage.Side)
			}
		}
	}
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

func postOrder(tokenID string, size int, limit float64) error {
	bigChainId := big.NewInt(chainId)
	orderBuilder := builder.NewExchangeOrderBuilderImpl(bigChainId, nil)
	makerAmount := int64(float64(size) * centsPerDollar * limit * baseTicks)
	takerAmount := int64(size) * int64(winnerTicks)
	orderData := model.OrderData{
		Maker: configuration.ProxyAddress,
		Signer: configuration.PolygonAddress,
		Taker: "0x0000000000000000000000000000000000000000",
		TokenId: tokenID,
		MakerAmount: strconv.FormatInt(makerAmount, 10),
		TakerAmount: strconv.FormatInt(takerAmount, 10),
		Side: model.BUY,
		Expiration: "0",
		Nonce: "0",
		FeeRateBps: "0",
	}
	orderModel, err := orderBuilder.BuildOrder(&orderData)
	if err != nil {
		log.Fatalf("Failed to build order: %v", err)
	}
	orderHash, err := orderBuilder.BuildOrderHash(orderModel, model.CTFExchange)
	if err != nil {
		log.Fatalf("Failed to build order hash: %v", err)
	}
	privateKeyString := configuration.PrivateKey
	if privateKeyString[:len(hexPrefix)] == hexPrefix {
		privateKeyString = privateKeyString[len(hexPrefix):]
	}
	privateKeyBytes := common.Hex2Bytes(privateKeyString)
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		log.Fatalf("Failed to create ECDSA: %v", err)
	}
	orderSignature, err := orderBuilder.BuildOrderSignature(privateKey, orderHash)
	if err != nil {
		log.Fatalf("Failed to build order signature: %v", err)
	}
	orderSignatureString := hexPrefix + common.Bytes2Hex(orderSignature)
	now := time.Now().UTC()
	timestamp := now.Unix()
	method := "POST"
	requestPath := "/order"
	order := Order{
		Salt: orderModel.Salt.Int64(),
		Maker: orderData.Maker,
		Signer: orderData.Signer,
		Taker: orderData.Taker,
		TokenID: orderData.TokenId,
		MakerAmount: orderData.MakerAmount,
		TakerAmount: orderData.TakerAmount,
		Side: "BUY",
		Expiration: orderData.Expiration,
		Nonce: orderData.Nonce,
		FeeRateBPs: orderData.FeeRateBps,
		SignatureType: 1,
		Signature: orderSignatureString,
	}
	newOrder := NewOrder{
		DeferExec: false,
		Order: order,
		Owner: configuration.APIKey,
		OrderType: "GTC",
	}
	bodyBytes, err := json.Marshal(newOrder)
	if err != nil {
		log.Fatalf("Failed to serialize order: %v", err)
	}
	body := string(bodyBytes)
	timestampString := strconv.FormatInt(timestamp, 10)
	message := timestampString + method + requestPath + body
	secretBytes, err := base64.StdEncoding.DecodeString(configuration.Secret)
	if err != nil {
		log.Fatalf("Failed to decode secret: %v", err)
	}
	hash := hmac.New(sha256.New, secretBytes)
	hash.Write([]byte(message))
	hashBytes := hash.Sum(nil)
	hmacSignature := base64.StdEncoding.EncodeToString(hashBytes)
	hmacSignature = strings.ReplaceAll(hmacSignature, "+", "-")
	hmacSignature = strings.ReplaceAll(hmacSignature, "/", "_")
	fmt.Printf("Order signature: %s\n", orderSignatureString)
	fmt.Printf("HMAC signature: %s\n", hmacSignature)
	url := "https://clob.polymarket.com/order"
	buffer := bytes.NewBuffer(bodyBytes)
	request, err := http.NewRequest(method, url, buffer)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("POLY_ADDRESS", configuration.PolygonAddress)
	request.Header.Set("POLY_API_KEY", configuration.APIKey)
	request.Header.Set("POLY_PASSPHRASE", configuration.Passphrase)
	request.Header.Set("POLY_SIGNATURE", hmacSignature)
	request.Header.Set("POLY_TIMESTAMP", timestampString)
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		log.Printf("Failed to POST order: %v", err)
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response after posting order: %v", err)
		return err
	}
	fmt.Printf("Response body: %s\n", responseBody)
	return nil
}