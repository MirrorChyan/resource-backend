package config

import (
	"bytes"
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"time"
)

const ServiceName = "resource-backend"

type watcher struct {
	*clientv3.Client
	waitIndex uint64
	key       string
}

func newConfigWatcher() *watcher {
	var (
		cfg = GConfig
	)
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{cfg.Registry.Endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	return &watcher{
		Client:    cli,
		waitIndex: 0,
		key:       fmt.Sprintf("%s/%s.%s", cfg.Registry.Path, DefaultConfigName, DefaultConfigType),
	}
}

func (p *watcher) pull() {
	resp, err := p.Get(context.Background(), p.key)
	if err != nil {
		log.Fatal(err)
	}
	triggerUpdate(func() error {
		return vp.MergeConfig(bytes.NewReader(resp.Kvs[0].Value))
	})
}

func (p *watcher) watch() {
	watch := p.Watch(context.Background(), p.key)
	for response := range watch {
		for _, ev := range response.Events {
			triggerUpdate(func() error {
				return vp.MergeConfig(bytes.NewReader(ev.Kv.Value))
			})
		}
	}
}
