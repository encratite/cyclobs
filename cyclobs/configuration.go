package cyclobs

import (
	"log"

	"gopkg.in/yaml.v3"
)

const configurationPath = "configuration/configuration.yaml"

type Configuration struct {
	Credentials Credentials `yaml:"credentials"`
	Live *bool `yaml:"live"`
	TagSlugs []string `yaml:"tagSlugs"`
	MinVolume *float64 `yaml:"minVolume"`
	OrderExpiration *int `yaml:"orderExpiration"`
	PositionLimit *int `yaml:"positionLimit"`
	Cleaner CleanerConfiguration `yaml:"cleaner"`
	Triggers []Trigger `yaml:"triggers"`
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
	Tolerance *float64 `yaml:"tolerance"`
}

type Trigger struct {
	TimeSpan *int `yaml:"timeSpan"`
	Delta *float64 `yaml:"delta"`
	MinPrice *float64 `yaml:"minPrice"`
	MaxPrice *float64 `yaml:"maxPrice"`
	Limit *float64 `yaml:"limit"`
	Size *int `yaml:"size"`
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
	if c.MinVolume == nil || *c.MinVolume < 1000 {
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
}

func (c *CleanerConfiguration) validate() {
	if c.Interval == nil || *c.Interval < 60 {
		log.Fatalf("Invalid interval in cleaner configuration")
	}
	if c.Expiration == nil || *c.Expiration < 60 {
		log.Fatalf("Invalid expiration in cleaner configuration")
	}
	if c.Tolerance == nil || *c.Tolerance < 0.01 || *c.Tolerance > 0.2 {
		log.Fatalf("Invalid tolerance in cleaner configuration")
	}
}

func (t *Trigger) validate() {
	if t.TimeSpan == nil || *t.TimeSpan < 60 {
		log.Fatalf("Invalid time span in trigger configuration")
	}
	if t.Delta == nil || *t.Delta < 0.01 || *t.Delta > 0.9 {
		log.Fatalf("Invalid delta in trigger configuration")
	}
	if t.MinPrice == nil || *t.MinPrice < 0.0 {
		log.Fatalf("Invalid min price in trigger configuration")
	}
	if t.MaxPrice == nil || *t.MaxPrice > 1.0 {
		log.Fatalf("Invalid max price in trigger configuration")
	}
	if t.Limit == nil || *t.Limit < 0.01 || *t.Limit > 0.99 {
		log.Fatalf("Invalid limit in trigger configuration")
	}
	if t.Size == nil || *t.Size < 5 || *t.Size > 1000 {
		log.Fatalf("Invalid position size in trigger configuration")
	}
}