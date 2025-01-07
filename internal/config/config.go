package config

import (
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Log      LogConfig      `mapstructure:"log"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
}

type AuthConfig struct {
	UploaderValidationURL string `mapstructure:"uploader_validation_url"`
	CDKValidationURL      string `mapstructure:"cdk_validation_url"`
}

const DefaultPort = 8000

func New() *Config {
	v := viper.New()

	v.SetDefault("server.port", DefaultPort)

	v.SetConfigName("config")
	v.SetConfigType("toml")

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory, %v", err)
	}
	// Search the current working directory first for the configuration file
	v.AddConfigPath(cwd)

	configDir := path.Join(cwd, "config")

	v.AddConfigPath(configDir)

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
