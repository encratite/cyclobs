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
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
)

const (
	chainId = 137
	centsPerDollar = 100
	ticksPerCent = int64(10000)
	hexPrefix = "0x"
	orderTypeGTC = "GTC"
	orderTypeGTD = "GTD"
)

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

type OrderResponse struct {
	ErrorMsg string `json:"errorMsg"`
	OrderID string `json:"orderID"`
	TakingAmount string `json:"takingAmount"`
	MakingAmount string `json:"makingAmount"`
	Status string `json:"status"`
	Success bool `json:"success"`
}

func postOrder(tokenID string, side model.Side, size int, limit float64, negRisk bool, expiration int) error {
	if len(tokenID) < 20 {
		log.Fatalf("Invalid tokenID")
	}
	if size < 5 {
		log.Fatalf("Invalid number of contracts: %d", size)
	}
	if limit < 0.01 {
		log.Fatalf("Invalid order limit: %.4f", limit)
	}
	if expiration < 0 {
		log.Fatalf("Invalid expiration: %d", expiration)
	}
	limit = float64(int(limit * centsPerDollar)) / float64(centsPerDollar)
	bigChainId := big.NewInt(chainId)
	orderBuilder := builder.NewExchangeOrderBuilderImpl(bigChainId, nil)
	makerAmount := int64(float64(size) * limit * centsPerDollar) * ticksPerCent
	takerAmount := int64(size * centsPerDollar) * ticksPerCent
	if side == model.SELL {
		makerSwap := takerAmount
		takerSwap := makerAmount
		makerAmount = makerSwap
		takerAmount = takerSwap
	}
	var expirationString string
	if expiration > 0 {
		expirationDuration := time.Duration(expiration) * time.Second
		expirationTime := time.Now().Add(expirationDuration).UTC()
		expirationString = intToString(expirationTime.Unix())
	} else {
		expirationString = "0"
	}
	orderData := model.OrderData{
		Maker: configuration.Credentials.ProxyAddress,
		Signer: configuration.Credentials.PolygonAddress,
		Taker: "0x0000000000000000000000000000000000000000",
		TokenId: tokenID,
		MakerAmount: intToString(makerAmount),
		TakerAmount: intToString(takerAmount),
		Side: side,
		Expiration: expirationString,
		Nonce: "0",
		FeeRateBps: "0",
		SignatureType: 1,
	}
	orderModel, err := orderBuilder.BuildOrder(&orderData)
	if err != nil {
		log.Fatalf("Failed to build order: %v", err)
	}
	contract := model.CTFExchange
	if negRisk {
		contract = model.NegRiskCTFExchange
	}
	orderHash, err := orderBuilder.BuildOrderHash(orderModel, contract)
	if err != nil {
		log.Fatalf("Failed to build order hash: %v", err)
	}
	privateKeyString := configuration.Credentials.PrivateKey
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
	sideString := "BUY"
	if side == model.SELL {
		sideString = "SELL"
	}
	order := Order{
		Salt: orderModel.Salt.Int64(),
		Maker: orderData.Maker,
		Signer: orderData.Signer,
		Taker: orderData.Taker,
		TokenID: orderData.TokenId,
		MakerAmount: orderData.MakerAmount,
		TakerAmount: orderData.TakerAmount,
		Side: sideString,
		Expiration: orderData.Expiration,
		Nonce: orderData.Nonce,
		FeeRateBPs: orderData.FeeRateBps,
		SignatureType: 1,
		Signature: orderSignatureString,
	}
	orderType := orderTypeGTC
	if expiration > 0 {
		orderType = orderTypeGTD
	}
	newOrder := NewOrder{
		DeferExec: false,
		Order: order,
		Owner: configuration.Credentials.APIKey,
		OrderType: orderType,
	}
	bodyBytes, err := json.Marshal(newOrder)
	if err != nil {
		log.Fatalf("Failed to serialize order: %v", err)
	}
	body := string(bodyBytes)
	timestampString := intToString(timestamp)
	message := timestampString + method + requestPath + body
	secretBytes, err := base64.StdEncoding.DecodeString(configuration.Credentials.Secret)
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
	request.Header.Set("POLY_ADDRESS", configuration.Credentials.PolygonAddress)
	request.Header.Set("POLY_API_KEY", configuration.Credentials.APIKey)
	request.Header.Set("POLY_PASSPHRASE", configuration.Credentials.Passphrase)
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
	log.Printf("Order response: %s\n", responseBody)
	var orderResponse OrderResponse
	err = json.Unmarshal(responseBody, &orderResponse)
	if err != nil {
		log.Printf("Failed to deserialize order response: %v", err)
		return err
	}
	if !orderResponse.Success {
		return fmt.Errorf("Failed to post order: %s", orderResponse.ErrorMsg)
	}
	return nil
}