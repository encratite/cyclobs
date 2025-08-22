package cyclobs

import (
	"log"

	"gopkg.in/yaml.v3"
)

const configurationPath = "configuration/configuration.yaml"

type Configuration struct {
	ProxyAddress string `yaml:"proxyAddress"`
	PrivateKey string `yaml:"privateKey"`
	PolygonAddress string `yaml:"polygonAddress"`
	APIKey string `yaml:"apiKey"`
	Secret string `yaml:"secret"`
	Passphrase string `yaml:"passphrase"`
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
}