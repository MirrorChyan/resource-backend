package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Log    LogConfig    `mapstructure:"log"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type LogConfig struct {
	Level      string `mapstructure:"level" toml:"level"`
	MaxSize    int    `mapstructure:"max_size" toml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" toml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" toml:"max_age"`
	Compress   bool   `mapstructure:"compress" toml:"compress"`
}

func New() *Config {
	v := viper.New()

	v.SetDefault("server.port", 8000)

	v.SetConfigName("config")
	v.SetConfigType("toml")

	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path, %v", err)
	}
	exeDir := filepath.Dir(exePath)
	v.AddConfigPath(exeDir)

	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file, %v", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		log.Fatalf("Failed to unmarshal config file, %v", err)
	}
	return &config
}
