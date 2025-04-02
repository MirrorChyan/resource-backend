package config

import (
	"github.com/bytedance/sonic"
	"log"
)

func doLoadRemoteConfig() {
	client := newConfigWatcher()
	client.pull()
	client.watch()
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
