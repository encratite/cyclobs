package cyclobs

import (
	"log"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

const configurationPath = "configuration/configuration.yaml"

type Configuration struct {
	Credentials Credentials `yaml:"credentials"`
	Live *bool `yaml:"live"`
	TagSlugs []string `yaml:"tagSlugs"`
	MinVolume *SerializableDecimal `yaml:"minVolume"`
	OrderExpiration *int `yaml:"orderExpiration"`
	PositionLimit *int `yaml:"positionLimit"`
	Cleaner CleanerConfiguration `yaml:"cleaner"`
	Triggers []Trigger `yaml:"triggers"`
	Database DatabaseConfiguration `yaml:"database"`
}

type Credentials struct {
	ProxyAddress string `yaml:"proxyAddress"`
	PrivateKey string `yaml:"privateKey"`
	PolygonAddress string `yaml:"polygonAddress"`
	APIKey string `yaml:"apiKey"`
	Secret string `yaml:"secret"`
	Passphrase string `yaml:"passphrase"`
}

type CleanerConfiguration struct {
	Interval *int `yaml:"interval"`
	Expiration *int `yaml:"expiration"`
	LimitOffset *SerializableDecimal `yaml:"limitOffset"`
}

type Trigger struct {
	TimeSpan *int `yaml:"timeSpan"`
	Delta *SerializableDecimal `yaml:"delta"`
	MinPrice *SerializableDecimal `yaml:"minPrice"`
	MaxPrice *SerializableDecimal `yaml:"maxPrice"`
	MinVolume *SerializableDecimal `yaml:"minVolume"`
	LimitOffset *SerializableDecimal `yaml:"limitOffset"`
	Size *int `yaml:"size"`
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
	if c.Live == nil {
		log.Fatalf("Live flag missing from configuration")
	}
	if c.TagSlugs == nil {
		log.Fatalf("Tag slugs missing from configuration")
	}
	minVolumeMin := decimal.NewFromInt(1000)
	if c.MinVolume == nil || c.MinVolume.LessThan(minVolumeMin) {
		log.Fatalf("Invalid min volume in configuration")
	}
	if c.OrderExpiration == nil || *c.OrderExpiration < 60 {
		log.Fatalf("Invalid order expiration in configuration")
	}
	if c.PositionLimit == nil || *c.PositionLimit < 1 {
		log.Fatalf("Invalid position limit in configuration")
	}
	c.Cleaner.validate()
	for _, trigger := range c.Triggers {
		trigger.validate()
	}
	c.Database.validate()
}

func (c *CleanerConfiguration) validate() {
	if c.Interval == nil || *c.Interval < 60 {
		log.Fatalf("Invalid interval in cleaner configuration")
	}
	if c.Expiration == nil || *c.Expiration < 60 {
		log.Fatalf("Invalid expiration in cleaner configuration")
	}
	toleranceMin := decimalConstant("0.01")
	toleranceMax := decimalConstant("0.2")
	if c.LimitOffset == nil || c.LimitOffset.LessThan(toleranceMin) || c.LimitOffset.GreaterThan(toleranceMax) {
		log.Fatalf("Invalid tolerance in cleaner configuration")
	}
}

func (t *Trigger) validate() {
	if t.TimeSpan == nil || *t.TimeSpan < 60 {
		log.Fatalf("Invalid time span in trigger configuration")
	}
	deltaMin := decimalConstant("0.01")
	deltaMax := decimalConstant("0.9")
	if t.Delta == nil || t.Delta.LessThan(deltaMin) || t.Delta.GreaterThan(deltaMax) {
		log.Fatalf("Invalid delta in trigger configuration")
	}
	priceMin := decimal.Zero
	priceMax := decimalConstant("1.0")
	if t.MinPrice == nil || t.MinPrice.LessThan(priceMin) || t.MinPrice.GreaterThan(priceMax) {
		log.Fatalf("Invalid min price in trigger configuration")
	}
	if t.MaxPrice == nil || t.MaxPrice.LessThan(priceMin) || t.MaxPrice.GreaterThan(priceMax) {
		log.Fatalf("Invalid max price in trigger configuration")
	}
	if t.MinPrice.GreaterThanOrEqual(t.MaxPrice.Decimal) {
		log.Fatalf("Min price must be less than max price")
	}
	minVolumeMin := decimal.NewFromInt(1000)
	minVolumeMax := decimal.NewFromInt(100000)
	if t.MinVolume == nil || t.MinVolume.LessThan(minVolumeMin) || t.MinVolume.GreaterThan(minVolumeMax) {
		log.Fatalf("Invalid min volume in trigger configuration")
	}
	limitOffsetMin := decimalConstant("0.01")
	limitOffsetMax := decimalConstant("0.99")
	if t.LimitOffset == nil || t.LimitOffset.LessThan(limitOffsetMin) || t.LimitOffset.GreaterThan(limitOffsetMax) {
		log.Fatalf("Invalid limit offset in trigger configuration")
	}
	if t.Size == nil || *t.Size < 5 || *t.Size > 1000 {
		log.Fatalf("Invalid position size in trigger configuration")
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