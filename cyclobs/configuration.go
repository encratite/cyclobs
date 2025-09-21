package cyclobs

import (
	"log"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

const configurationPath = "configuration/configuration.yaml"

type Configuration struct {
	Credentials Credentials `yaml:"credentials"`
	Data DataModeConfiguration `yaml:"data"`
	Database DatabaseConfiguration `yaml:"database"`
	Trigger TriggerModeConfiguration `yaml:"trigger"`
	Jump JumpConfiguration `yaml:"jump"`
}

type Credentials struct {
	ProxyAddress string `yaml:"proxyAddress"`
	PrivateKey string `yaml:"privateKey"`
	PolygonAddress string `yaml:"polygonAddress"`
	APIKey string `yaml:"apiKey"`
	Secret string `yaml:"secret"`
	Passphrase string `yaml:"passphrase"`
}

type DataModeConfiguration struct {
	TagSlugs []string `yaml:"tagSlugs"`
	Events []string `yaml:"events"`
	MinVolume *SerializableDecimal `yaml:"minVolume"`
	BufferTimeSpan *int `yaml:"bufferTimeSpan"`
}

type TriggerModeConfiguration struct {
	Live *bool `yaml:"live"`
	RecordData *bool `yaml:"recordData"`
	Triggers []Trigger `yaml:"triggers"`
}

type Trigger struct {
	Slug *string `yaml:"slug"`
	TakeProfit *SerializableDecimal `yaml:"takeProfit"`
	TakeProfitLimit *SerializableDecimal `yaml:"takeProfitLimit"`
	StopLoss *SerializableDecimal `yaml:"stopLoss"`
	StopLossLimit *SerializableDecimal `yaml:"stopLossLimit"`
}

type JumpConfiguration struct {
	Threshold1 *SerializableDecimal `yaml:"threshold1"`
	Threshold2 *SerializableDecimal `yaml:"threshold2"`
	Threshold3 *SerializableDecimal `yaml:"threshold3"`
	SpreadLimit *SerializableDecimal `yaml:"spreadLimit"`
	IncludeTags []string `yaml:"includeTags"`
	ExcludeTags []string `yaml:"excludeTags"`
}

type DatabaseConfiguration struct {
	URI *string `yaml:"uri"`
	Database *string `yaml:"database"`
}

type SerializableDecimal struct {
	decimal.Decimal
}

var configuration *Configuration

func loadConfiguration() {
	if configuration != nil {
		panic("Configuration had already been loaded")
	}
	yamlData := readFile(configurationPath)
	configuration = new(Configuration)
	err := yaml.Unmarshal(yamlData, configuration)
	if err != nil {
		log.Fatal("Failed to unmarshal YAML:", err)
	}
	configuration.validate()
}

func (c *Configuration) validate() {
	c.Data.validate()
	c.Database.validate()
	c.Trigger.validate()
}

func (c *DataModeConfiguration) validate() {
	if c.TagSlugs == nil {
		log.Fatalf("Tag slugs missing from data mode configuration")
	}
	minVolumeMin := decimal.NewFromInt(1000)
	if c.MinVolume == nil || c.MinVolume.LessThan(minVolumeMin) {
		log.Fatalf("Invalid min volume in data mode configuration")
	}
	if c.BufferTimeSpan == nil || *c.BufferTimeSpan < 3600 {
		log.Fatalf("Invalid buffer time span in data mode configuration")
	}
}

func (c *TriggerModeConfiguration) validate() {
	if c.Live == nil {
		log.Fatalf("Live flag missing from configuration")
	}
	if c.RecordData == nil {
		log.Fatalf("Record data flag missing from configuration")
	}
	for _, trigger := range c.Triggers {
		trigger.validate()
	}
}

func (c *JumpConfiguration) validate() {
	thresholds := []*SerializableDecimal{
		c.Threshold1,
		c.Threshold2,
		c.Threshold3,
	}
	priceMin := decimal.Zero
	priceMax := decimalConstant("1.0")
	for _, threshold := range thresholds {
		if threshold == nil {
			log.Fatalf("Missing threshold in jump configuration")
		}
		if threshold.LessThanOrEqual(priceMin) || threshold.GreaterThanOrEqual(priceMax) {
			log.Fatalf("Invalid threshold in jump configuration")
		}
	}
	if c.SpreadLimit == nil {
		log.Fatalf("Spread limit missing from jump configuration")
	}
	if c.SpreadLimit.IsNegative() {
		log.Fatalf("Spread limit can't be negative")
	}
}

func (t *Trigger) validate() {
	if t.Slug == nil {
		log.Fatalf("Slug missing from trigger configuration")
	}
	priceMin := decimal.Zero
	priceMax := decimalConstant("1.0")
	if t.TakeProfit != nil {
		if t.TakeProfit.LessThanOrEqual(priceMin) || t.TakeProfit.GreaterThanOrEqual(priceMax) {
			log.Fatalf("Invalid take profit price in trigger configuration")
		}
		if t.StopLoss.GreaterThanOrEqual(t.TakeProfit.Decimal) {
			log.Fatalf("Stop-loss must be less than take profit price")
		}
		if t.TakeProfitLimit == nil || t.TakeProfitLimit.LessThanOrEqual(priceMin) || t.TakeProfitLimit.GreaterThanOrEqual(priceMax) {
			log.Fatalf("Invalid take profit limit in trigger configuration")
		}
	}
	if t.StopLoss == nil || t.StopLoss.LessThanOrEqual(priceMin) || t.StopLoss.GreaterThanOrEqual(priceMax) {
		log.Fatalf("Invalid stop-loss price in trigger configuration")
	}
	if t.StopLossLimit == nil || t.StopLossLimit.LessThanOrEqual(priceMin) || t.StopLossLimit.GreaterThanOrEqual(priceMax) {
		log.Fatalf("Invalid stop-loss limit in trigger configuration")
	}
}

func (c *DatabaseConfiguration) validate() {
	if c.URI == nil {
		log.Fatalf("MongoDB URI missing in configuration file")
	}
	if c.Database == nil {
		log.Fatalf("MongoDB database missing in configuration file")
	}
}

func (d *SerializableDecimal) UnmarshalYAML(value *yaml.Node) error {
	decimalValue, err := decimal.NewFromString(value.Value)
	if err != nil {
		return err
	}
	d.Decimal = decimalValue
	return nil
}