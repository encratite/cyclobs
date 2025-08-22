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
	Market string `json:"market"`
	AssetID string `json:"asset_id"`
	Timestamp string `json:"timestamp"`
	Hash string `json:"hash"`
	Bids []OrderSummary `json:"bids"`
	Asks []OrderSummary `json:"asks"`
	Changes []PriceChange `json:"changes"`
	EventType string `json:"event_type"`
	FeeRateBPs string `json:"fee_rate_bps"`
	Price string `json:"price"`
	Side string `json:"side"`
	Size string `json:"size"`
}

type OrderSummary struct {
	Price string `json:"price"`
	Size string `json:"size"`
}

type PriceChange struct {
	Price string `json:"price"`
	Side string `json:"side"`
	Size string `json:"size"`
}