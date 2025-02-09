package config

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

var (
	GConfig = new(Config)
	vp      *viper.Viper
)

func InitGlobalConfig() {
	doLoadLocalConfig()
	if GConfig.Instance.OnlyLocal {
		log.Println("Use Standalone mode")
		return
	}
	log.Println("Use Cluster mode")
	doLoadRemoteConfig()
}

func doLoadLocalConfig() {
	vp = viper.New()
	vp.SetConfigName(DefaultConfigName)
	vp.SetConfigType(DefaultConfigType)
	vp.AddConfigPath(".")
	vp.AddConfigPath("config")

	if err := vp.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config file, %v", err)
	}

	if err := vp.Unmarshal(GConfig); err != nil {
		log.Fatalf("Failed to unmarshal config file, %v", err)
	}

	supplyExtraConfig()
}

func supplyExtraConfig() {
	if GConfig.Instance.OnlyLocal {
		return
	}
	ip, ok := os.LookupEnv(instanceIp)
	if !ok {
		panic("please set environment variable " + instanceIp)
	}
	id, ok := os.LookupEnv(serviceId)
	if !ok {
		panic("please set environment variable " + serviceId)
	}
	rid, ok := os.LookupEnv(regionId)
	if !ok {
		rid = "default"
	}

	GConfig.Instance.RegionId = rid
	GConfig.Instance.Address = ip
	GConfig.Instance.Port = DefaultPort
	GConfig.Registry.ServiceId = id

}
