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
	Id string `json:"id"`
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
	ConditionID string `json:"condition_id"`
	QuestionID string `json:"question_id"`
	Tokens []Token `json:"tokens"`
	Rewards Rewards `json:"rewards"`
	MinimumOrderSize string `json:"minimum_order_size"`
	MinimumTickSize string `json:"minimum_tick_size"`
	Category string `json:"category"`
	EndDateISO string `json:"end_date_iso"`
	GameStartTime string `json:"game_start_time"`
	Question string `json:"question"`
	MarketSlug string `json:"market_slug"`
	MinIncentiveSize string `json:"min_incentive_size"`
	MaxIncentiveSize string `json:"max_incentive_size"`
	Active bool `json:"active"`
	Closed bool `json:"closed"`
	SecondsDelay int `json:"seconds_delay"`
	Icon string `json:"icon"`
	FPMM string `json:"fpmm"`
}

type Tag struct {
	Id string `json:"id"`
	Label string `json:"label"`
	Slug string `json:"slug"`
	ForceShow bool `json:"forceShow"`
	CreatedAt string `json:"createdAt"`
}

type Token struct {
	TokenID string `json:"token_id"`
	Outcome string `json:"outcome"`
}

type Rewards struct {
	MinSize float64 `json:"min_size"`
	MaxSpread float64 `json:"max_spread"`
	EventStartDate string `json:"event_start_date"`
	EventEndDate string `json:"event_end_date"`
	InGameMultiplier float64 `json:"in_game_multiplier"`
	RewardEpoch float64 `json:"reward_epoch"`
}