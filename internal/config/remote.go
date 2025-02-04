package config

import (
	"bytes"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
)

func loadRemoteConfig(v *viper.Viper, config *Config) {
	var (
		registry = config.Registry
	)
	clientConfig := constant.NewClientConfig(
		constant.WithNamespaceId(registry.NamespaceId),
		constant.WithUsername(registry.Username),
		constant.WithPassword(registry.Password),
		constant.WithTimeoutMs(5000),
		constant.WithLogLevel("debug"),
		constant.WithLogDir(os.TempDir()),
		constant.WithCacheDir(os.TempDir()),
		constant.WithNotLoadCacheAtStart(true),
	)
	zap.L().Info(" - Parsing Config For Nacos")
	client, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig: clientConfig,
		ServerConfigs: []constant.ServerConfig{
			{
				IpAddr:   registry.Host,
				Port:     registry.Port,
				GrpcPort: registry.GrpcPort,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	c, err := client.GetConfig(vo.ConfigParam{
		Group:  registry.Group,
		DataId: registry.DataId,
	})
	if err != nil {
		panic(err)
	}
	if err := v.MergeConfig(bytes.NewReader([]byte(c))); err != nil {
		zap.L().Fatal("Error Parsed, Check Your Remote Config Syntax %v ", zap.Error(err))
		panic(err)
	}
	if err := v.Unmarshal(&config); err != nil {
		zap.L().Fatal("Failed to unmarshal remote config, %v", zap.Error(err))
		panic(err)
	}

	err = client.ListenConfig(vo.ConfigParam{
		Group:  registry.Group,
		DataId: registry.DataId,
		OnChange: func(namespace, group, dataId, data string) {
			if err := v.MergeConfig(bytes.NewReader([]byte(data))); err != nil {
				zap.L().Error("Update Config Error", zap.Error(err))
				return
			}
			origin := config.Log.Level
			if e := v.Unmarshal(&config); e != nil {
				zap.L().Error("Update Config Error", zap.Error(e))
				return
			}
			if origin != config.Log.Level && levelListener != nil {
				levelListener(config.Log.Level)
			}

			zap.L().Info("Nacos Config Update")
		},
	})
	if err != nil {
		panic(err)
	}
}

func SetLogLevelChangeListener(listener func(level string)) {
	levelListener = listener
}
