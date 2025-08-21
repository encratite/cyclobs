package cyclobs

type Subscription struct {
	Auth *Auth `json:"auth"`
	Markets *[]string `json:"markets"`
	AssetIDs *[]string `json:"assets_ids"`
	Type string `json:"string"`
}

type Auth struct {
	APIKey string `json:"apiKey"`
	APISecret string `json:"secret"`
	PassPhrase string `json:"passphrase"`
}

type BookMessage struct {
	EventType string `json:"event_type"`
	AssetID string `json:"asset_id"`
	Market string `json:"market"`
	Timestamp string `json:"timestamp"`
	Hash string `json:"hash"`
	Buys []OrderSummary `json:"buys"`
	Sells []OrderSummary `json:"sells"`
}

type OrderSummary struct {
	Price string `json:"price"`
	Size string `json:"size"`
}