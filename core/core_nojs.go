//go:build !js

package core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_badger "github.com/aperturerobotics/hydra/volume/badger"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	volume_redis "github.com/aperturerobotics/hydra/volume/redis"
	volume_sqlite "github.com/aperturerobotics/hydra/volume/sqlite"
)

// addNativeFactories adds factories specific to this platform.
func addNativeFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_badger.NewFactory(b))
	sr.AddFactory(volume_bolt.NewFactory(b))
	sr.AddFactory(volume_redis.NewFactory(b))
	sr.AddFactory(volume_sqlite.NewFactory(b))
}
