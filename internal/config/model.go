package config

import "time"

type (
	Config struct {
		Instance InstanceConfig `mapstructure:"instance"`
		Log      LogConfig      `mapstructure:"log"`
		Registry Registration   `mapstructure:"registry"`
		Database DatabaseConfig `mapstructure:"database"`
		Auth     AuthConfig     `mapstructure:"auth"`
		Redis    RedisConfig    `mapstructure:"redis"`
		Extra    ExtraConfig    `mapstructure:"extra"`
	}
	InstanceConfig struct {
		Address string
		Port    int `mapstructure:"port"`
		// only_local is used to indicate whether to only use local config
		OnlyLocal bool   `mapstructure:"only_local"`
		RegionId  string `mapstructure:"region_id"`
	}

	Registration struct {
		Endpoint  string `mapstructure:"endpoint"`
		Path      string `mapstructure:"path"`
		ServiceId string `mapstructure:"service_id"`
	}

	LogConfig struct {
		Level string `mapstructure:"level"`
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
		DownloadPrefixInfo      map[string][]RobinServer `mapstructure:"download_prefix_info"`
		DownloadEffectiveTime   time.Duration            `mapstructure:"download_effective_time"`
		DownloadLimitCount      int64                    `mapstructure:"download_limit_count"`
		DownloadRedirectPrefix  string                   `mapstructure:"download_redirect_prefix"`
		SqlDebugMode            bool                     `mapstructure:"sql_debug_mode"`
		CreateNewVersionWebhook string                   `mapstructure:"create_new_version_webhook"`
		CdnPrefix               string                   `mapstructure:"cdn_prefix"`
		DistributeCdnRatio      int                      `mapstructure:"distribute_cdn_ratio"`
		DistributeCdnRegion     []string                 `mapstructure:"distribute_cdn_region"`
		Concurrency             int32                    `mapstructure:"concurrency"`
	}

	RobinServer struct {
		Url    string `mapstructure:"url"`
		Weight int    `mapstructure:"weight"`
	}

	AuthConfig struct {
		SignSecret            string `mapstructure:"sign_secret"`
		PrivateKey            string `mapstructure:"private_key"`
		UploaderValidationURL string `mapstructure:"uploader_validation_url"`
		CDKValidationURL      string `mapstructure:"cdk_validation_url"`
	}
)
