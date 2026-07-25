// Package publiccdk holds the open-access cdk whitelist.
//
// An open-access cdk is resolved locally instead of being sent to the upstream
// validation service, so /latest answers it exactly like a paid one, but the
// distribute info is marked and the download key is refused on redemption.
package publiccdk

import (
	"cmp"
	"sync/atomic"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"go.uber.org/zap"
)

const listenKey = "auth.public_cdk"

type entry struct {
	all       bool
	resources map[string]struct{}
	expiredAt int64
}

type snapshot struct {
	enabled bool
	keys    map[string]*entry
}

var current atomic.Pointer[snapshot]

func init() {
	config.RegisterKeyListener(config.KeyListener{
		Key: listenKey,
		Listener: func(any) {
			Reload()
		},
	})
}

// Reload rebuilds the whitelist from config.GConfig.
//
// The remote config listener only fires when the consul value actually changes,
// so this must also be called once explicitly after config.InitGlobalConfig,
// otherwise the standalone mode would always see an empty whitelist.
func Reload() {
	var (
		conf = config.GConfig.Auth.PublicCDK
		s    = &snapshot{
			enabled: conf.Enabled,
			keys:    make(map[string]*entry, len(conf.Keys)),
		}
	)

	for _, k := range conf.Keys {
		if k.Key == "" {
			continue
		}
		e := &entry{
			resources: make(map[string]struct{}, len(k.Resources)),
			expiredAt: cmp.Or(k.ExpiredTime, conf.ExpiredTime),
		}
		if len(k.Resources) == 0 {
			e.all = true
		}
		for _, r := range k.Resources {
			if r == "*" {
				e.all = true
				continue
			}
			e.resources[r] = struct{}{}
		}
		s.keys[k.Key] = e
	}

	current.Store(s)

	zap.L().Info("public cdk reloaded",
		zap.Bool("enabled", s.enabled),
		zap.Int("count", len(s.keys)),
	)
}

// Match reports whether cdk is an open-access cdk of the given resource,
// it returns the expired time exposed as cdk_expired_time when matched.
func Match(cdk, rid string) (int64, bool) {
	s := current.Load()
	if s == nil || !s.enabled || cdk == "" {
		return 0, false
	}
	e, ok := s.keys[cdk]
	if !ok {
		return 0, false
	}
	if !e.all {
		if _, ok := e.resources[rid]; !ok {
			return 0, false
		}
	}
	return e.expiredAt, true
}
