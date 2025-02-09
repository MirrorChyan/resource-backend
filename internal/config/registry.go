package config

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const ServiceName = "resource-backend"

func NewConsulClient() *api.Client {
	var (
		cfg    = GConfig
		option = api.DefaultConfig()
	)
	option.Address = cfg.Registry.Endpoint

	client, err := api.NewClient(option)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func doRegisterService(client *api.Client) {
	var (
		cfg = GConfig
		sid = cfg.Registry.ServiceId
	)
	service := &api.AgentServiceRegistration{
		ID:      sid,
		Name:    ServiceName,
		Port:    cfg.Instance.Port,
		Address: cfg.Instance.Address,
		Check: &api.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://%s:%d/health", cfg.Instance.Address, cfg.Instance.Port),
			Interval: "10s",
		},
	}

	if err := client.Agent().ServiceRegister(service); err != nil {
		log.Fatal(err)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case sig := <-ch:
				zap.L().Info("Catch Signal", zap.String("signal", sig.String()), zap.String("ServiceDeregister", sid))
				_ = client.Agent().ServiceDeregister(sid)
				os.Exit(0)
			}
		}
	}()
}
