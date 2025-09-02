package cyclobs

type EventsResponse struct {
	Data []Event `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	HasMore bool `json:"hasMore"`
	TotalResults int `json:"totalResults"`
}

type Event struct {
	ID string `json:"id"`
	Ticker string `json:"ticker"`
	Slug string `json:"slug"`
	Title string `json:"title"`
	Description string `json:"description"`
	ResolutionSource string `json:"resolutionSource"`
	StartDate string `json:"startDate"`
	CreationDate string `json:"creationDate"`
	EndDate string `json:"endDate"`
	Image string `json:"image"`
	Icon string `json:"icon"`
	Active bool `json:"active"`
	Closed bool `json:"closed"`
	Archived bool `json:"archived"`
	New bool `json:"new"`
	Featured bool `json:"featured"`
	Restricted bool `json:"restricted"`
	Liquidity float64 `json:"liquidity"`
	Volume float64 `json:"volume"`
	OpenInterest int `json:"openInterest"`
	SortBy string `json:"sortBy"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Competitive float64 `json:"competitive"`
	Volume24Hr float64 `json:"volume24hr"`
	Volume1Wk float64 `json:"volume1Wk"`
	Volume1Mo float64 `json:"volume1mo"`
	Volume1Yr float64 `json:"volume1yr"`
	EnableOrderBook bool `json:"enableOrderBook"`
	LiquidityClob float64 `json:"liquidityClob"`
	NegRisk bool `json:"negRisk"`
	CommentCount int `json:"commentCount"`
	Markets []Market `json:"markets"`
	Tags []Tag `json:"tags"`
	CYOM bool `json:"cyom"`
	ShowAllOutcomes bool `json:"showAllOutcomes"`
	ShowMarketImages bool `json:"showMarketImages"`
	EnableNegRisk bool `json:"enableNegRisk"`
	AutomaticallyActive bool `json:"automaticallyActive"`
	GMPChartMode string `json:"gmpChartMode"`
	NegRiskAugmented bool `json:"negRiskAugmented"`
	PendingDeployment bool `json:"pendingDeployment"`
	Deploying bool `json:"deploying"`
}

type Market struct {
	ID string `json:"id"`
	Question string `json:"question"`
	ConditionID string `json:"conditionId"`
	Slug string `json:"slug"`
	EndDate string `json:"endDate"`
	Liquidity string `json:"liquidity"`
	StartDate string `json:"startDate"`
	Image string `json:"image"`
	Icon string `json:"icon"`
	Description string `json:"description"`
	Outcomes string `json:"outcomes"`
	OutcomePrices string `json:"outcomePrices"`
	Volume string `json:"volume"`
	Active bool `json:"active"`
	Closed bool `json:"closed"`
	MarketMakerAddress string `json:"marketMakerAddress"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	New bool `json:"new"`
	Featured bool `json:"featured"`
	SubmittedBy string `json:"submitted_by"`
	Archived bool `json:"archived"`
	ResolvedBy string `json:"resolvedBy"`
	Restricted bool `json:"restricted"`
	GroupItemTitle string `json:"groupItemTitle"`
	GroupItemThreshold string `json:"groupItemThreshold"`
	EnableOrderBook bool `json:"enableOrderBook"`
	OrderPriceMinTickSize float64 `json:"orderPriceMinTickSize"`
	OrderMinSize int `json:"orderMinSize"`
	VolumeNum float64 `json:"volumeNum"`
	LiquidityNum float64 `json:"liquidityNum"`
	EndDateISO string `json:"endDateIso"`
	StartDateISO string `json:"startDateIso"`
	HasReviewedDates bool `json:"hasReviewedDates"`
	Volume24Hr float64 `json:"volume24hr"`
	Volume1Wk float64 `json:"volume1wk"`
	Volume1Mo float64 `json:"volume1mo"`
	Volume1Yr float64 `json:"volume1yr"`
	CLOBTokenIDs string `json:"clobTokenIds"`
	UMABond string `json:"umaBond"`
	UMAReward string `json:"umaReward"`
	Volume24HrCLOB float64 `json:"volume24hrClob"`
	Volume1WkCLOB float64 `json:"volume1wkClob"`
	Volume1MoCLOB float64 `json:"volume1moClob"`
	Volume1YrCLOB float64 `json:"volume1yrClob"`
	VolumeCLOB float64 `json:"volumeClob"`
	LiquidityCLOB float64 `json:"liquidityClob"`
	AcceptingOrders bool `json:"acceptingOrders"`
	NegRisk bool `json:"negRisk"`
	Events []Event `json:"events"`
	NegRiskMarketID string `json:"negRiskMarketID"`
	NegRiskRequestID string `json:"negRiskRequestID"`
	Ready bool `json:"ready"`
	Funded bool `json:"funded"`
	AcceptingOrdersTimestamp string `json:"acceptingOrdersTimestamp"`
	CYOM bool `json:"cyom"`
	Competitive float64 `json:"competitive"`
	PagerDutyNotificationEnabled bool `json:"pagerDutyNotificationEnabled"`
	Approved bool `json:"approved"`
	CLOBRewards []CLOBReward `json:"clobRewards"`
	RewardsMinSize int `json:"rewardsMinSize"`
	RewardsMaxSpread float64 `json:"rewardsMaxSpread"`
	Spread float64 `json:"spread"`
	OneDayPriceChange float64 `json:"oneDayPriceChange"`
	OneHourPriceChange float64 `json:"oneHourPriceChange"`
	OneWeekPriceChange float64 `json:"oneWeekPriceChange"`
	OneMonthPriceChange float64 `json:"oneMonthPriceChange"`
	LastTradePrice float64 `json:"lastTradePrice"`
	BestBid float64 `json:"bestBid"`
	BestAsk float64 `json:"bestAsk"`
	AutomaticallyActive bool `json:"automaticallyActive"`
	ClearBookOnStart bool `json:"clearBookOnStart"`
	ShowGMPSeries bool `json:"showGmpSeries"`
	ShowGMPOutcome bool `json:"showGmpOutcome"`
	ManualActivation bool `json:"manualActivation"`
	NegRiskOther bool `json:"negRiskOther"`
	UMAResolutionStatuses string `json:"umaResolutionStatuses"`
	PendingDeployment bool `json:"pendingDeployment"`
	Deploying bool `json:"deploying"`
	DeployingTimestamp string `json:"deployingTimestamp"`
	RFQEnabled bool `json:"rfqEnabled"`
	HoldingRewardsEnabled bool `json:"holdingRewardsEnabled"`
}

type Tag struct {
	ID string `json:"id"`
	Label string `json:"label"`
	Slug string `json:"slug"`
	ForceShow bool `json:"forceShow"`
	CreatedAt string `json:"createdAt"`
}

type Token struct {
	TokenID string `json:"token_id"`
	Outcome string `json:"outcome"`
}

type CLOBReward struct {
	ID string `json:"id"`
	ConditionID string `json:"conditionId"`
	AssetAddress string `json:"assetAddress"`
	RewardsAmount int `json:"rewardsAmount"`
	RewardsDailyRate int `json:"rewardsDailyRate"`
	StartDate string `json:"startDate"`
	EndDate string `json:"endDate"`
}

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

type Position struct {
	ProxyWallet string `json:"proxyWallet"`
	Asset string `json:"asset"`
	ConditionID string `json:"conditionId"`
	Size float64 `json:"size"`
	AvgPrice float64 `json:"avgPrice"`
	InitialValue float64 `json:"initialValue"`
	CurrentValue float64 `json:"currentValue"`
	CashPnL float64 `json:"cashPnl"`
	PercentPnL float64 `json:"percentPnl"`
	TotalBought float64 `json:"totalBought"`
	RealizedPnL float64 `json:"realizedPnl"`
	PercentRealizedPnL float64 `json:"percentRealizedPnl"`
	CurPrice float64 `json:"curPrice"`
	Redeemable bool `json:"redeemable"`
	Mergeable bool `json:"mergeable"`
	Title string `json:"title"`
	Slug string `json:"slug"`
	Icon string `json:"icon"`
	EventSlug string `json:"eventSlug"`
	Outcome string `json:"outcome"`
	OutcomeIndex int `json:"outcomeIndex"`
	OppositeOutcome string `json:"oppositeOutcome"`
	OppositeAsset string `json:"oppositeAsset"`
	EndDate string `json:"endDate"`
	NegativeRisk bool `json:"negativeRisk"`
}

type PriceHistory struct {
	History []PriceHistorySample `json:"history"`
}

type PriceHistorySample struct {
	Time int `json:"t"`
	Price float64 `json:"p"`
}

type EventTag struct {
	ID string `json:"id"`
	Label string `json:"label"`
	Slug string `json:"slug"`
	ForceShow bool `json:"forceShow"`
	PublishedAt string `json:"publishedAt"`
	CreatedBy int `json:"createdBy"`
	UpdatedBy int `json:"updatedBy"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}