package config

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/bytedance/sonic"
	"github.com/hashicorp/consul/api"
)

func doLoadRemoteConfig() {
	var (
		cfg    = GConfig
		client = NewConsulClient()
		path   = cfg.Registry.Path
	)

	poll := poller{client.KV(), 0}
	poll.pollRemoteConfig(path)

	doRegisterService(client)

}

var c = sonic.Config{
	SortMapKeys: true,
}.Froze()

func triggerUpdate(update func() error) {
	var (
		origin      = *GConfig
		originValue []any
	)
	for _, l := range listeners {
		val := vp.Get(l.Key)
		str, err := c.MarshalToString(val)
		if err != nil {
			log.Printf("failed to marshal val, %v\n", err)
		}
		originValue = append(originValue, str)
	}

	if err := update(); err != nil {
		log.Printf("failed to dynamic update config file, %v\n", err)
		return
	}

	if err := vp.Unmarshal(GConfig); err != nil {
		GConfig = &origin
		log.Printf("failed to dynamic update config file, %v\n", err)
	}

	for i, l := range listeners {
		val := vp.Get(l.Key)
		str, err := c.MarshalToString(val)
		if err != nil {
			log.Printf("failed to marshal val, %v\n", err)
		}
		if str != originValue[i] && l.Listener != nil {
			l.Listener(val)
		}

	}

}

type poller struct {
	store     *api.KV
	waitIndex uint64
}

func (p *poller) pollRemoteConfig(path string) {
	var key = fmt.Sprintf("%s/%s.%s", path, DefaultConfigName, DefaultConfigType)
	go func() {
		for {
			keypair, meta, err := p.store.Get(key, &api.QueryOptions{
				WaitIndex: p.waitIndex,
			})
			if keypair == nil && err == nil {
				err = fmt.Errorf("key ( %s ) was not found", key)
			}
			if err != nil {
				log.Println("Remote Config Update Error", err)
				time.Sleep(time.Second * 5)
				continue
			}
			p.waitIndex = meta.LastIndex
			log.Println("Remote Config Update")
			triggerUpdate(func() error {
				return vp.MergeConfig(bytes.NewReader(keypair.Value))
			})
		}
	}()

}
