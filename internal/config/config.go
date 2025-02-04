package config

import (
	"github.com/spf13/viper"
	"log"
)

var (
	CFG           *Config
	levelListener func(level string)
)

func New() *Config {
	v, c := loadLocalConfig()
	loadRemoteConfig(v, c)
	return c
}

func loadLocalConfig() (*viper.Viper, *Config) {
	v := viper.New()
	v.SetDefault(ServerPortKey, DefaultPort)
	v.SetConfigName(DefaultConfigName)
	v.SetConfigType(DefaultConfigType)
	v.AddConfigPath(".")
	v.AddConfigPath("config")

	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file, %v", err)
	}

	var c = new(Config)
	if err := v.Unmarshal(c); err != nil {
		log.Fatalf("Failed to unmarshal config file, %v", err)
	}

	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file, %v", err)
	}
	return v, c
}
