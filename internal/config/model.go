package config

import "time"

type (
	Config struct {
		Server   ServerConfig   `mapstructure:"server"`
		Log      LogConfig      `mapstructure:"log"`
		Registry Registration   `mapstructure:"registry"`
		Database DatabaseConfig `mapstructure:"database"`
		Auth     AuthConfig     `mapstructure:"auth"`
		Billing  BillingConfig  `mapstructure:"billing"`
		Redis    RedisConfig    `mapstructure:"redis"`
		Extra    ExtraConfig    `mapstructure:"extra"`
	}
	ServerConfig struct {
		Port int `mapstructure:"port"`
	}

	Registration struct {
		Host        string `mapstructure:"host"`
		Port        uint64 `mapstructure:"port"`
		GrpcPort    uint64 `mapstructure:"grpc_port"`
		NamespaceId string `mapstructure:"namespace_id"`
		Group       string `mapstructure:"group"`
		DataId      string `mapstructure:"data_id"`
		Username    string `mapstructure:"username"`
		Password    string `mapstructure:"password"`
	}

	LogConfig struct {
		Level      string `mapstructure:"level"`
		MaxSize    int    `mapstructure:"max_size"`
		MaxBackups int    `mapstructure:"max_backups"`
		MaxAge     int    `mapstructure:"max_age"`
		Compress   bool   `mapstructure:"compress"`
	}
	DatabaseConfig struct {
		Host     string `mapstructure:"host"`
		Port     string `mapstructure:"port"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
		Name     string `mapstructure:"name"`
	}
	RedisConfig struct {
		Addr     string `mapstructure:"addr"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
		Name     string `mapstructure:"name"`
		DB       int    `mapstructure:"db"`
	}
	ExtraConfig struct {
		DownloadPrefix        []string      `mapstructure:"download_prefix"`
		DownloadEffectiveTime time.Duration `mapstructure:"download_effective_time"`
		SqlDebugMode          bool          `mapstructure:"sql_debug_mode"`
	}
	AuthConfig struct {
		UploaderValidationURL string `mapstructure:"uploader_validation_url"`
		CDKValidationURL      string `mapstructure:"cdk_validation_url"`
	}
	BillingConfig struct {
		CheckinURL string `mapstructure:"billing_checkin_url"`
	}
)
