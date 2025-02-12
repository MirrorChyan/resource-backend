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

	worker := poller{
		store:     client.KV(),
		waitIndex: 0,
		key:       fmt.Sprintf("%s/%s.%s", path, DefaultConfigName, DefaultConfigType),
	}
	worker.loadRemoteConfigImmediately()
	worker.doPollRemoteConfig()

	doRegisterService(client)

}
func (p *poller) loadRemoteConfigImmediately() {

	keypair, _, err := p.store.Get(p.key, &api.QueryOptions{})
	if err != nil {
		log.Fatal("Load Remote Config Error", err)
	}

	triggerUpdate(func() error {
		if err = vp.MergeConfig(bytes.NewReader(keypair.Value)); err != nil {
			log.Fatal("MergeConfig Remote Config Error", err)
		}
		return nil
	})

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
			log.Println("Remote Config Update")
			l.Listener(val)
		}

	}

}

type poller struct {
	store     *api.KV
	waitIndex uint64
	key       string
}

func (p *poller) doPollRemoteConfig() {
	go func() {
		for {
			keypair, meta, err := p.store.Get(p.key, &api.QueryOptions{
				WaitIndex: p.waitIndex,
			})
			if keypair == nil && err == nil {
				err = fmt.Errorf("key ( %s ) was not found", p.key)
			}
			if err != nil {
				log.Println("Remote Config Update Error", err)
				time.Sleep(time.Second * 5)
				continue
			}
			p.waitIndex = meta.LastIndex
			triggerUpdate(func() error {
				return vp.MergeConfig(bytes.NewReader(keypair.Value))
			})
		}
	}()

}
